//go:build cxfx

package cxfx

import (
	"bufio"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t0-util" // thin: only ProgramFileSystemPathFromBytes, ProgramSandboxFileOpen; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"

	//"bytes"
	"unsafe"
)

func Slice_ui8_ToPtr(value []uint8) unsafe.Pointer {
	count := len(value)
	if count == 0 {
		return unsafe.Pointer(nil)
	}
	return unsafe.Pointer(&value[0])
}

func Slice_ui32_ToPtr(value []uint32) unsafe.Pointer {
	count := len(value)
	if count == 0 {
		return unsafe.Pointer(nil)
	}
	return unsafe.Pointer(&value[0])
}

func Slice_i32_ToPtr(value []int32) unsafe.Pointer {
	count := len(value)
	if count == 0 {
		return unsafe.Pointer(nil)
	}
	return unsafe.Pointer(&value[0])
}

func Slice_f32_ToPtr(value []float32) unsafe.Pointer {
	count := len(value)
	if count == 0 {
		return unsafe.Pointer(nil)
	}
	return unsafe.Pointer(&value[0])
}

// VIOLATION STRHOT-325: B, decodes an image file, whose format the decoder names, #432.
func decodeImg(file *os.File, cpuCopy bool) (data []byte, width int32, height int32, pixels []float32) {
	img, _, err := image.Decode(file)
	if err != nil {
		panic(err)
	}

	rgba := image.NewRGBA(img.Bounds())
	if rgba.Stride != rgba.Rect.Size().X*4 {
		panic("unsupported stride")
	}

	draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)
	data = rgba.Pix
	width = int32(rgba.Rect.Size().X)
	height = int32(rgba.Rect.Size().Y)
	if cpuCopy {
		pixels = make([]float32, width*height*4)
		var x int32
		var y int32
		for y = 0; y < height; y++ {
			yoffset := y * width * 4
			for x = 0; x < width; x++ {
				var xoffset = yoffset + x*4
				color := rgba.At(int(x), int(y))
				r, g, b, a := color.RGBA()
				pixels[xoffset] = float32(r) / 65535.0
				pixels[xoffset+1] = float32(g) / 65535.0
				pixels[xoffset+2] = float32(b) / 65535.0
				pixels[xoffset+3] = float32(a) / 65535.0
			}
		}
	}
	return
}

const (
	HDR_NONE = iota
	HDR_32_RLE_RGBE
	MINLEN = 8
	MAXLEN = 0x7fff
	R      = 0
	G      = 1
	B      = 2
	E      = 3
)

func unpack(file *os.File, width types.Pointer, line []byte) bool {
	if width < MINLEN || width > MAXLEN {
		return unpack_(file, width, line)
	}

	file.Read(line[:4])
	if line[R] != 2 {
		file.Seek(-4, io.SeekCurrent)
		return unpack_(file, width, line)
	}

	if line[G] != 2 || (line[B]&128) != 0 {
		return unpack_(file, width-1, line[4:])
	}

	var b [1]byte
	for i := types.Pointer(0); i < 4; i++ {
		for j := types.Pointer(0); j < width; {
			file.Read(b[:])
			count := types.Pointer(b[0])
			if count > 128 {
				count &= 127
				file.Read(b[:])
				var value int = int(b[0])
				for c := types.Pointer(0); c < count; c++ {
					line[j+c+i] = byte(value)
				}
			} else {
				for c := types.Pointer(0); c < count; c++ {
					offset := j + c + i
					file.Read(line[offset : offset+1])
				}
			}
		}
	}
	return true
}

func unpack_(file *os.File, width types.Pointer, line []byte) bool {
	var rshift uint
	var repeat [4]byte
	for width > 0 {
		file.Read(line[0:4])
		if line[R] == 1 && line[G] == 1 && line[B] == 1 {
			for i := line[E] << rshift; i > 0; i-- {
				copy(line[0:4], repeat[:])
				line = line[4:]
				width--
			}
			rshift += 8
		} else {
			copy(repeat[:], line[0:4])
			line = line[4:]
			width--
			rshift = 0
		}
	}
	return true
}

// VIOLATION STRHOT-197: B, parses the text header of an HDR file, #432.
func decodeHdr(file *os.File) (data []byte, i32Width int32, i32Height int32) {
	data = nil
	i32Width = 0
	i32Height = 0

	var format int
	scanner := bufio.NewScanner(file)

	var pos int64
	scanLines := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		advance, token, err = bufio.ScanLines(data, atEOF)
		pos += int64(advance)
		return
	}

	scanner.Split(scanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "#?RADIANCE" {
		} else if strings.HasPrefix(line, "#") {
		} else if strings.HasPrefix(line, "FORMAT=") {
			var sformat string
			if n, err := fmt.Sscanf(line, "FORMAT=%s\n", &sformat); n != 1 && err != nil {
				fmt.Printf("Failed to scan format : err '%s'\n", err)
				return
			}
			if sformat == "32-bit_rle_rgbe" {
				format = HDR_32_RLE_RGBE
			}
		} else if len(line) == 0 {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Failed to scan : err %v\n", scanner.Err())
	}

	if format != HDR_32_RLE_RGBE {
		fmt.Printf("Invalid format %d\n", format)
		return
	}

	file.Seek(pos, 0)

	var iwidth int
	var iheight int
	if n, err := fmt.Fscanf(file, "-Y %d +X %d\n", &iwidth, &iheight); n != 2 || err != nil {
		fmt.Printf("Failed to scan width and height : err '%s'\n", err)
		return
	}

	i32Width = int32(iwidth)
	i32Height = int32(iheight)

	width := types.Cast_int_to_ptr(iwidth)
	height := types.Cast_int_to_ptr(iheight)

	//var colors []float32 = make([]float32, width*height*3)
	var line []byte = make([]byte, width*4)
	data = make([]byte, width*height*3*4)

	for y := types.Pointer(0); y < height; y++ {
		if unpack(file, width, line) == false {
			fmt.Printf("Failed to unpack line %d\n", y)
			return
		}

		yoffset := y /*(height - y - 1)*/ * width * 3 * 4
		for x := types.Pointer(0); x < width; x++ {
			loffset := x * 4
			exponent := math.Pow(2.0, float64(int(line[loffset+3])-128))
			xoffset := yoffset + x*3*4
			r := float32(exponent * float64(line[loffset]) / 256.0)
			g := float32(exponent * float64(line[loffset+1]) / 256.0)
			b := float32(exponent * float64(line[loffset+2]) / 256.0)

			types.Write_f32(data, xoffset, r)
			types.Write_f32(data, xoffset+4, g)
			types.Write_f32(data, xoffset+8, b)
		}
	}
	return
}

// uploadTexture loads the program path in pathBytes from the sandbox into the
// bound texture. With cpuCopy it keeps the pixels and returns their handle
// into CxfxState.CxfxTexturePixelsTable, otherwise -1.
// VIOLATION STRHOT-198: B, reads the file at a program path and names it in the error text, #432.
func uploadTexture(prgrm *ast.CXProgram, pathBytes []byte, target uint32, level uint32, cpuCopy bool) int32 {
	path := util.ProgramTextStringView(pathBytes)
	programPath, fileError := util.ProgramFileSystemPathFromBytes(pathBytes)
	if fileError != nil {
		panic(fmt.Sprintf("texture %q: %s\n", path, fileError.FileErrorMessage))
	}
	file, fileError := util.ProgramSandboxFileOpen(prgrm.ProgramSandboxRoot, programPath)
	if fileError != nil {
		panic(fmt.Sprintf("texture %q not found on disk: %s\n", path, fileError.FileErrorMessage))
	}
	defer file.Close()

	pixelsHandle := int32(-1)
	ext := filepath.Ext(path)
	var data []byte
	var internalFormat int32
	var inputFormat uint32
	var inputType uint32
	var width int32
	var height int32
	var pixels []float32
	if ext == ".png" || ext == ".jpeg" || ext == ".jpg" {
		internalFormat = cxglRGBA8
		inputFormat = cxglRGBA
		inputType = cxglUNSIGNED_BYTE
		data, width, height, pixels = decodeImg(file, cpuCopy)
		if len(pixels) > 0 {
			pixelsHandle = cxfxTexturePixelsAdd(CxfxTexturePixels{
				CxfxTexturePixelsWidth:  width,
				CxfxTexturePixelsHeight: height,
				CxfxTexturePixelsLevel:  level,
				CxfxTexturePixelsData:   pixels,
			})
		}
	} else if ext == ".hdr" {
		internalFormat = cxglRGB16F
		inputFormat = cxglRGB
		inputType = cxglFLOAT
		data, width, height = decodeHdr(file)
	}

	if len(data) > 0 {
		cxglTexImage2D(target, int32(level), internalFormat, width, height, 0, inputFormat, inputType, data)
	}
	return pixelsHandle
}

// gogl
func OpGlNewTexture(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var texture uint32
	cxglEnable(cxglTEXTURE_2D)
	cxglGenTextures(1, &texture)
	cxglBindTexture(cxglTEXTURE_2D, texture)
	cxglTexParameteri(cxglTEXTURE_2D, cxglTEXTURE_MIN_FILTER, cxglNEAREST)
	cxglTexParameteri(cxglTEXTURE_2D, cxglTEXTURE_MAG_FILTER, cxglNEAREST)
	cxglTexParameteri(cxglTEXTURE_2D, cxglTEXTURE_WRAP_S, cxglCLAMP_TO_EDGE)
	cxglTexParameteri(cxglTEXTURE_2D, cxglTEXTURE_WRAP_T, cxglCLAMP_TO_EDGE)

	uploadTexture(prgrm, ast.ProgramStringBytesRead(prgrm, &inputs[0]), cxglTEXTURE_2D, 0, false)

	outputs[0].Set_i32(prgrm, int32(texture))
}

// OpGlNewTextureCube loads the six faces pattern+posx+extension to
// pattern+negz+extension.
// GCREACH: builds each face path in a new byte slice, once per face, see
// docs/GarbageCollector.md.
func OpGlNewTextureCube(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var texture uint32
	cxglEnable(cxglTEXTURE_CUBE_MAP)
	cxglGenTextures(1, &texture)
	cxglBindTexture(cxglTEXTURE_CUBE_MAP, texture)
	cxglTexParameteri(cxglTEXTURE_CUBE_MAP, cxglTEXTURE_MIN_FILTER, cxglNEAREST)
	cxglTexParameteri(cxglTEXTURE_CUBE_MAP, cxglTEXTURE_MAG_FILTER, cxglNEAREST)
	cxglTexParameteri(cxglTEXTURE_CUBE_MAP, cxglTEXTURE_WRAP_S, cxglCLAMP_TO_EDGE)
	cxglTexParameteri(cxglTEXTURE_CUBE_MAP, cxglTEXTURE_WRAP_T, cxglCLAMP_TO_EDGE)
	cxglTexParameteri(cxglTEXTURE_CUBE_MAP, cxglTEXTURE_WRAP_R, cxglCLAMP_TO_EDGE)

	var faces = [6][4]byte{{'p', 'o', 's', 'x'}, {'n', 'e', 'g', 'x'}, {'p', 'o', 's', 'y'}, {'n', 'e', 'g', 'y'}, {'p', 'o', 's', 'z'}, {'n', 'e', 'g', 'z'}}
	pattern := ast.ProgramStringBytesRead(prgrm, &inputs[0])
	extension := ast.ProgramStringBytesRead(prgrm, &inputs[1])
	for i := 0; i < 6; i++ {
		path := make([]byte, 0, len(pattern)+len(faces[i])+len(extension))
		path = append(append(append(path, pattern...), faces[i][:]...), extension...)
		uploadTexture(prgrm, path, uint32(cxglTEXTURE_CUBE_MAP_POSITIVE_X+i), 0, false)
	}
	outputs[0].Set_i32(prgrm, int32(texture))
}

func OpCxReleaseTexture(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	*cxfxTexturePixelsGet(inputs[0].Get_i32(prgrm)) = CxfxTexturePixels{}
}

func OpCxTextureGetPixel(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	texture := cxfxTexturePixelsGet(inputs[0].Get_i32(prgrm))
	x := inputs[1].Get_i32(prgrm)
	y := inputs[2].Get_i32(prgrm)
	offset := (y*texture.CxfxTexturePixelsWidth + x) * 4
	pixels := texture.CxfxTexturePixelsData
	outputs[0].Set_f32(prgrm, pixels[offset])
	outputs[1].Set_f32(prgrm, pixels[offset+1])
	outputs[2].Set_f32(prgrm, pixels[offset+2])
	outputs[3].Set_f32(prgrm, pixels[offset+3])
}

func OpGlUploadImageToTexture(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, uploadTexture(prgrm, ast.ProgramStringBytesRead(prgrm, &inputs[0]), uint32(inputs[1].Get_i32(prgrm)), uint32(inputs[2].Get_i32(prgrm)), inputs[3].Get_bool(prgrm)))
}

// OpGlNewGIF decodes the gif at a program path and returns its handle into
// CxfxState.CxfxGifTable with its frame count, loop count and size.
// VIOLATION STRHOT-204: B, reads the file at a program path and names it in the error text, #432.
func OpGlNewGIF(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	path := util.ProgramTextStringView(ast.ProgramStringBytesRead(prgrm, &inputs[0]))
	file, fileError := ast.ProgramSandboxFileOpen(prgrm, &inputs[0])
	if fileError != nil {
		panic(fmt.Sprintf("file not found %q, %s", path, fileError.FileErrorMessage))
	}
	defer file.Close()

	animation, err := gif.DecodeAll(bufio.NewReader(file))
	if err != nil {
		panic(fmt.Sprintf("failed to decode file %q, %v", path, err))
	}

	outputs[0].Set_i32(prgrm, cxfxGifAdd(animation))
	outputs[1].Set_i32(prgrm, int32(len(animation.Image)))
	outputs[2].Set_i32(prgrm, int32(animation.LoopCount))
	outputs[3].Set_i32(prgrm, int32(animation.Config.Width))
	outputs[4].Set_i32(prgrm, int32(animation.Config.Height))
}

func OpGlFreeGIF(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	handle := inputs[0].Get_i32(prgrm)
	cxfxGifGet(handle)
	cxfxState.CxfxGifTable[handle] = nil
}

// GCREACH: draws the frame into a new RGBA image, see docs/GarbageCollector.md.
func OpGlGIFFrameToTexture(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	animation := cxfxGifGet(inputs[0].Get_i32(prgrm))
	frame := inputs[1].Get_i32(prgrm)
	texture := inputs[2].Get_i32(prgrm)

	img := animation.Image[frame]
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)

	cxglBindTexture(cxglTEXTURE_2D, uint32(texture))
	cxglTexImage2D(cxglTEXTURE_2D, 0, cxglRGBA, int32(rgba.Rect.Size().X), int32(rgba.Rect.Size().Y), 0, cxglRGBA, cxglUNSIGNED_BYTE, rgba.Pix)

	outputs[0].Set_i32(prgrm, int32(animation.Delay[frame]))
	outputs[1].Set_i32(prgrm, int32(animation.Disposal[frame]))
}

// GCREACH: allocates on the heap, which adds to the GC pressure,
// see docs/GarbageCollector.md.
func OpGlAppend(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputSlicePointer := outputs[0].Offset
	outputSliceOffset := types.Read_ptr(prgrm.Memory, outputSlicePointer)

	inputSliceOffset := ast.GetSliceOffset(prgrm, inputs[0].FramePointer, inputs[0].TypeSignature)
	var inputSliceLen types.Pointer
	if inputSliceOffset != 0 {
		inputSliceLen = ast.GetSliceLen(prgrm, inputSliceOffset)
	}

	obj := inputs[1].Get_bytes(prgrm)

	objLen := types.Cast_int_to_ptr(len(obj))
	outputSliceOffset = ast.SliceResizeEx(prgrm, outputSliceOffset, inputSliceLen+objLen, 1)
	ast.SliceCopyEx(prgrm, outputSliceOffset, inputSliceOffset, inputSliceLen, 1)
	ast.SliceAppendWriteByte(prgrm, outputSliceOffset, obj, inputSliceLen)
	outputs[0].Set_ptr(prgrm, outputSliceOffset)
}

// gl_1_0
func OpGlCullFace(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglCullFace(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlFrontFace(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglFrontFace(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlHint(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglHint(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlScissor(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglScissor(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_i32(prgrm),
		inputs[3].Get_i32(prgrm))
}

func OpGlTexParameteri(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglTexParameteri(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		inputs[2].Get_i32(prgrm))
}

func OpGlTexImage2D(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglTexImage2D(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_i32(prgrm),
		inputs[3].Get_i32(prgrm),
		inputs[4].Get_i32(prgrm),
		inputs[5].Get_i32(prgrm),
		uint32(inputs[6].Get_i32(prgrm)),
		uint32(inputs[7].Get_i32(prgrm)),
		inputs[8].GetSlice_bytes(prgrm))
}

func OpGlClear(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglClear(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlClearColor(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglClearColor(
		inputs[0].Get_f32(prgrm),
		inputs[1].Get_f32(prgrm),
		inputs[2].Get_f32(prgrm),
		inputs[3].Get_f32(prgrm))
}

func OpGlClearStencil(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglClearStencil(inputs[0].Get_i32(prgrm))
}

func OpGlClearDepth(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglClearDepth(inputs[0].Get_f64(prgrm))
}

func OpGlStencilMask(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglStencilMask(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlColorMask(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglColorMask(
		inputs[0].Get_bool(prgrm),
		inputs[1].Get_bool(prgrm),
		inputs[2].Get_bool(prgrm),
		inputs[3].Get_bool(prgrm))
}

func OpGlDepthMask(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDepthMask(inputs[0].Get_bool(prgrm))
}

func OpGlDisable(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDisable(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlEnable(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglEnable(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlBlendFunc(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBlendFunc(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlStencilFunc(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglStencilFunc(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		uint32(inputs[2].Get_i32(prgrm)))
}

func OpGlStencilOp(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglStencilOp(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		uint32(inputs[2].Get_i32(prgrm)))
}

func OpGlDepthFunc(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDepthFunc(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlGetError(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(cxglGetError()))
}

func OpGlGetTexLevelParameteriv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var outValue int32 = 0
	cxglGetTexLevelParameteriv(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		uint32(inputs[2].Get_i32(prgrm)),
		&outValue)
	outputs[0].Set_i32(prgrm, outValue)
}

func OpGlDepthRange(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDepthRange(
		inputs[0].Get_f64(prgrm),
		inputs[1].Get_f64(prgrm))
}

func OpGlViewport(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglViewport(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_i32(prgrm),
		inputs[3].Get_i32(prgrm))
}

// gl_1_1
func OpGlDrawArrays(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDrawArrays(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_i32(prgrm))
}

func OpGlDrawElements(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDrawElements(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		uint32(inputs[2].Get_i32(prgrm)),
		nil)
}

func OpGlBindTexture(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBindTexture(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlDeleteTextures(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglDeleteTextures(inputs[0].Get_i32(prgrm), &inpV1) // will panic if inp0 > 1
}

func OpGlGenTextures(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglGenTextures(inputs[0].Get_i32(prgrm), &inpV1) // will panic if inp0 > 1
	outputs[0].Set_i32(prgrm, int32(inpV1))
}

// gl_1_3
func OpGlActiveTexture(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglActiveTexture(uint32(inputs[0].Get_i32(prgrm)))
}

// gl_1_4
func OpGlBlendFuncSeparate(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBlendFuncSeparate(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		uint32(inputs[2].Get_i32(prgrm)),
		uint32(inputs[3].Get_i32(prgrm)))
}

// gl_1_5
func OpGlBindBuffer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBindBuffer(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlDeleteBuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglDeleteBuffers(
		inputs[0].Get_i32(prgrm),
		&inpV1) // will panic if inp0 > 1
}

func OpGlGenBuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglGenBuffers(
		inputs[0].Get_i32(prgrm),
		&inpV1) // will panic if inp0 > 1
	outputs[0].Set_i32(prgrm, int32(inpV1))
}

func OpGlBufferData(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBufferData(
		uint32(inputs[0].Get_i32(prgrm)),
		int(inputs[1].Get_i32(prgrm)),
		inputs[2].GetSlice_bytes(prgrm),
		uint32(inputs[3].Get_i32(prgrm)))
}

func OpGlBufferSubData(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBufferSubData(
		uint32(inputs[0].Get_i32(prgrm)),
		int(inputs[1].Get_i32(prgrm)),
		int(inputs[2].Get_i32(prgrm)),
		inputs[3].GetSlice_bytes(prgrm))
}

func OpGlDrawBuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDrawBuffers(
		inputs[0].Get_i32(prgrm),
		inputs[1].GetSlice_bytes(prgrm))
}

func OpGlStencilOpSeparate(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglStencilOpSeparate(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		uint32(inputs[2].Get_i32(prgrm)),
		uint32(inputs[3].Get_i32(prgrm)))
}

func OpGlStencilFuncSeparate(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglStencilFuncSeparate(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		inputs[2].Get_i32(prgrm),
		uint32(inputs[3].Get_i32(prgrm)))
}

func OpGlStencilMaskSeparate(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglStencilMaskSeparate(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlAttachShader(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglAttachShader(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlBindAttribLocation(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBindAttribLocation(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		ast.ProgramStringBytesRead(prgrm, &inputs[2]))
}

func OpGlCompileShader(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	shader := uint32(inputs[0].Get_i32(prgrm))
	cxglCompileShader(shader)
}

func OpGlCreateProgram(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(cxglCreateProgram()))
}

func OpGlCreateShader(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int32(cxglCreateShader(uint32(inputs[0].Get_i32(prgrm))))
	outputs[0].Set_i32(prgrm, outV0)
}

func OpGlDeleteProgram(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDeleteShader(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlDeleteShader(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDeleteShader(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlDetachShader(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglDetachShader(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlEnableVertexAttribArray(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglEnableVertexAttribArray(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlGetAttribLocation(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := cxglGetAttribLocation(
		uint32(inputs[0].Get_i32(prgrm)),
		ast.ProgramStringBytesRead(prgrm, &inputs[1]))
	outputs[0].Set_i32(prgrm, outV0)
}

func OpGlGetProgramiv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := cxglGetProgramiv(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
	outputs[0].Set_i32(prgrm, outV0)
}

func OpGlGetProgramInfoLog(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := cxglGetProgramInfoLog(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm))
	outputs[0].Set_str_bytes(prgrm, outV0)
}

func OpGlGetShaderiv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := cxglGetShaderiv(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
	outputs[0].Set_i32(prgrm, outV0)
}

func OpGlGetShaderInfoLog(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := cxglGetShaderInfoLog(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm))
	outputs[0].Set_str_bytes(prgrm, outV0)
}

func OpGlGetUniformLocation(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := cxglGetUniformLocation(
		uint32(inputs[0].Get_i32(prgrm)),
		ast.ProgramStringBytesRead(prgrm, &inputs[1]))
	outputs[0].Set_i32(prgrm, outV0)
}

func OpGlLinkProgram(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	program := uint32(inputs[0].Get_i32(prgrm))
	cxglLinkProgram(program)
}

func OpGlShaderSource(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglShaderSource(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		ast.ProgramStringBytesRead(prgrm, &inputs[2]))
}

func OpGlUseProgram(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUseProgram(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlUniform1f(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform1f(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_f32(prgrm))
}

func OpGlUniform2f(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform2f(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_f32(prgrm),
		inputs[2].Get_f32(prgrm))
}

func OpGlUniform3f(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform3f(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_f32(prgrm),
		inputs[2].Get_f32(prgrm),
		inputs[3].Get_f32(prgrm))
}

func OpGlUniform4f(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform4f(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_f32(prgrm),
		inputs[2].Get_f32(prgrm),
		inputs[3].Get_f32(prgrm),
		inputs[4].Get_f32(prgrm))
}

func OpGlUniform1i(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform1i(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm))
}

func OpGlUniform2i(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform2i(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_i32(prgrm))
}

func OpGlUniform3i(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform3i(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_i32(prgrm),
		inputs[3].Get_i32(prgrm))
}

func OpGlUniform4i(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform4i(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_i32(prgrm),
		inputs[3].Get_i32(prgrm),
		inputs[4].Get_i32(prgrm))
}

func OpGlUniform1fv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform1fv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].GetSlice_bytes(prgrm))
}

func OpGlUniform2fv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform2fv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].GetSlice_bytes(prgrm))
}

func OpGlUniform3fv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform3fv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].GetSlice_bytes(prgrm))
}

func OpGlUniform4fv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform4fv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].GetSlice_bytes(prgrm))
}

func OpGlUniform1iv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform1iv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].GetSlice_bytes(prgrm))
}

func OpGlUniform2iv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform2iv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].GetSlice_bytes(prgrm))
}

func OpGlUniform3iv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform3iv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].GetSlice_bytes(prgrm))
}

func OpGlUniform4iv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform4iv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].GetSlice_bytes(prgrm))
}

func OpGlUniformMatrix2fv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniformMatrix2fv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_bool(prgrm),
		inputs[3].GetSlice_bytes(prgrm))
}

func OpGlUniformMatrix3fv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniformMatrix3fv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_bool(prgrm),
		inputs[3].GetSlice_bytes(prgrm))
}

func OpGlUniformMatrix4fv(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniformMatrix4fv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_bool(prgrm),
		inputs[3].GetSlice_bytes(prgrm))
}

func OpGlUniformV4F(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniform4fv(
		inputs[0].Get_i32(prgrm),
		1,
		inputs[1].Get_bytes(prgrm))
}

func OpGlUniformM44F(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniformMatrix4fv(
		inputs[0].Get_i32(prgrm),
		1,
		inputs[1].Get_bool(prgrm),
		inputs[2].Get_bytes(prgrm))
}

func OpGlUniformM44FV(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglUniformMatrix4fv(
		inputs[0].Get_i32(prgrm),
		inputs[1].Get_i32(prgrm),
		inputs[2].Get_bool(prgrm),
		inputs[3].GetSlice_bytes(prgrm))
}

func OpGlVertexAttribPointer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglVertexAttribPointer(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		uint32(inputs[2].Get_i32(prgrm)),
		inputs[3].Get_bool(prgrm),
		inputs[4].Get_i32(prgrm), 0)
}

func OpGlVertexAttribPointerI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglVertexAttribPointer(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		uint32(inputs[2].Get_i32(prgrm)),
		inputs[3].Get_bool(prgrm),
		inputs[4].Get_i32(prgrm),
		inputs[5].Get_i32(prgrm))
}

func OpGlClearBufferI(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	color := [4]int32{
		inputs[2].Get_i32(prgrm),
		inputs[3].Get_i32(prgrm),
		inputs[4].Get_i32(prgrm),
		inputs[5].Get_i32(prgrm)}

	cxglClearBufferiv(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		color[:])
}

func OpGlClearBufferUI(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	color := [4]uint32{
		inputs[2].Get_ui32(prgrm),
		inputs[3].Get_ui32(prgrm),
		inputs[4].Get_ui32(prgrm),
		inputs[5].Get_ui32(prgrm)}

	cxglClearBufferuiv(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		color[:])
}

func OpGlClearBufferF(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	color := [4]float32{
		inputs[2].Get_f32(prgrm),
		inputs[3].Get_f32(prgrm),
		inputs[4].Get_f32(prgrm),
		inputs[5].Get_f32(prgrm)}

	cxglClearBufferfv(
		uint32(inputs[0].Get_i32(prgrm)),
		inputs[1].Get_i32(prgrm),
		color[:])
}

func OpGlBindRenderbuffer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBindRenderbuffer(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlDeleteRenderbuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglDeleteRenderbuffers(inputs[0].Get_i32(prgrm), &inpV1) // will panic if inp0 > 1
}

func OpGlGenRenderbuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglGenRenderbuffers(inputs[0].Get_i32(prgrm), &inpV1) // will panic if inp0 > 1
	outputs[0].Set_i32(prgrm, int32(inpV1))
}

func OpGlRenderbufferStorage(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglRenderbufferStorage(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		inputs[2].Get_i32(prgrm),
		inputs[3].Get_i32(prgrm))
}

func OpGlBindFramebuffer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBindFramebuffer(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)))
}

func OpGlDeleteFramebuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglDeleteFramebuffers(inputs[0].Get_i32(prgrm), &inpV1) // will panic if inp0 > 1
}

func OpGlGenFramebuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglGenFramebuffers(inputs[0].Get_i32(prgrm), &inpV1) // will panic if inp0 > 1
	outputs[0].Set_i32(prgrm, int32(inpV1))
}

func OpGlCheckFramebufferStatus(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int32(cxglCheckFramebufferStatus(uint32(inputs[0].Get_i32(prgrm))))
	outputs[0].Set_i32(prgrm, outV0)
}

func OpGlFramebufferTexture2D(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglFramebufferTexture2D(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		uint32(inputs[2].Get_i32(prgrm)),
		uint32(inputs[3].Get_i32(prgrm)),
		inputs[4].Get_i32(prgrm))
}

func OpGlFramebufferRenderbuffer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglFramebufferRenderbuffer(
		uint32(inputs[0].Get_i32(prgrm)),
		uint32(inputs[1].Get_i32(prgrm)),
		uint32(inputs[2].Get_i32(prgrm)),
		uint32(inputs[3].Get_i32(prgrm)))
}

func OpGlGenerateMipmap(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglGenerateMipmap(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlBindVertexArray(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := uint32(inputs[0].Get_i32(prgrm))
	if runtime.GOOS == "darwin" {
		cxglBindVertexArrayAPPLE(inpV0)
	} else {
		cxglBindVertexArray(inpV0)
	}
}

func OpGlBindVertexArrayCore(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxglBindVertexArray(uint32(inputs[0].Get_i32(prgrm)))
}

func OpGlDeleteVertexArrays(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_i32(prgrm)
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	if runtime.GOOS == "darwin" {
		cxglDeleteVertexArraysAPPLE(inpV0, &inpV1) // will panic if inp0 > 1
	} else {
		cxglDeleteVertexArrays(inpV0, &inpV1) // will panic if inp0 > 1
	}
}

func OpGlDeleteVertexArraysCore(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglDeleteVertexArrays(inputs[0].Get_i32(prgrm), &inpV1) // will panic if inp0 > 1
}

func OpGlGenVertexArrays(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_i32(prgrm)
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	if runtime.GOOS == "darwin" {
		cxglGenVertexArraysAPPLE(inpV0, &inpV1) // will panic if inp0 > 1
	} else {
		cxglGenVertexArrays(inpV0, &inpV1) // will panic if inp0 > 1
	}
	outputs[0].Set_i32(prgrm, int32(inpV1))
}

func OpGlGenVertexArraysCore(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV1 := uint32(inputs[1].Get_i32(prgrm))
	cxglGenVertexArrays(inputs[0].Get_i32(prgrm), &inpV1) // will panic if inp0 > 1
	outputs[0].Set_i32(prgrm, int32(inpV1))
}
