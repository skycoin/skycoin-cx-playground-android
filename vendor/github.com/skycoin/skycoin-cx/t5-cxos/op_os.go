//go:build !nocxos

package cxos

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t0-util"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

const (
	OS_SEEK_SET = iota
	OS_SEEK_CUR
	OS_SEEK_END
)

// VIOLATION: state outside State. Owner: t5-cxos; moves to OsFileMng in phase 1.
var openFiles []*os.File

// VIOLATION: state outside State. Owner: t5-cxos; moves to OsFileMng in phase 1.
var freeFiles []int32

// helper function used to validate file handle from i32
func ValidFile(handle int32) *os.File {
	if handle >= 0 && handle < int32(len(openFiles)) && openFiles[handle] != nil {
		return openFiles[handle]
	}
	return nil
}

func opOsLogFile(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	util.CXLogFile(inputs[0].Get_bool(prgrm))
}

// VIOLATION STRHOT-131: B, hands a path or text to Go or the OS, #412.
func opOsReadAllText(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false

	if byts, fileError := ast.ProgramSandboxFileRead(prgrm, &inputs[0]); fileError == nil {
		outputs[0].Set_str(prgrm, string(byts))
		success = true
	}

	outputs[1].Set_bool(prgrm, success)
}

func getFileHandle(file *os.File) int32 {
	handle := int32(-1)
	freeCount := len(freeFiles)
	if freeCount > 0 {
		freeCount--
		handle = int32(freeFiles[freeCount])
		freeFiles = freeFiles[:freeCount]
	} else {
		handle = int32(len(openFiles))
		openFiles = append(openFiles, nil)
	}

	if handle < 0 || handle >= int32(len(openFiles)) {
		panic("internal error")
	}

	openFiles[handle] = file
	return handle
}

func opOsOpen(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	handle := int32(-1)
	if file, fileError := ast.ProgramSandboxFileOpen(prgrm, &inputs[0]); fileError == nil {
		handle = getFileHandle(file)
	}

	outputs[0].Set_i32(prgrm, int32(handle))
}

func opOsCreate(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	handle := int32(-1)
	if file, fileError := ast.ProgramSandboxFileCreate(prgrm, &inputs[0]); fileError == nil {
		handle = getFileHandle(file)
	}

	outputs[0].Set_i32(prgrm, int32(handle))
}

func opOsClose(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, fileClose(inputs[0].Get_i32(prgrm)))
}

// fileClose closes the file of handle and frees the handle. It returns
// false for a handle that is not open or a failed close.
func fileClose(handle int32) bool {
	success := false
	if file := ValidFile(handle); file != nil {
		if err := file.Close(); err == nil {
			success = true
		}

		openFiles[handle] = nil
		freeFiles = append(freeFiles, handle)
	}
	return success
}

// libraryFileCloseVm is os.Close on vm, an i32 at block and the bool after it.
func libraryFileCloseVm(memory []byte, block uint32) {
	closed := fileClose(types.Read_i32(memory, types.Pointer(block)))
	types.Write_bool(memory, types.Pointer(block+4), closed)
}

func opOsSeek(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i64(prgrm, fileSeek(inputs[0].Get_i32(prgrm), inputs[1].Get_i64(prgrm), inputs[2].Get_i32(prgrm)))
}

// fileSeek moves the file of handle to offset from whence and returns the
// new offset, -1 for a handle that is not open or a failed seek.
func fileSeek(handle int32, offset int64, whence int32) int64 {
	file := ValidFile(handle)
	if file == nil {
		return -1
	}
	position, err := file.Seek(offset, int(whence))
	if err != nil {
		return -1
	}
	return position
}

// libraryFileSeekVm is os.Seek on vm, an i32, an i64 and an i32 from block
// and the i64 after them.
func libraryFileSeekVm(memory []byte, block uint32) {
	position := fileSeek(types.Read_i32(memory, types.Pointer(block)), types.Read_i64(memory, types.Pointer(block+4)), types.Read_i32(memory, types.Pointer(block+12)))
	types.Write_i64(memory, types.Pointer(block+16), position)
}

// VIOLATION STRHOT-134: B, hands a path or text to Go or the OS, #412.
func opOsReadStr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var len uint64
	var value string
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &len); err == nil {
			bytes := make([]byte, len)
			if err := binary.Read(file, binary.LittleEndian, &bytes); err == nil {
				value = string(bytes)
				success = true
			}
		}
	}
	outputs[0].Set_str(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadF64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value float64
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_f64(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadF32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value float32
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_f32(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadUI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value uint64
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_ui64(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadUI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value uint32
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_ui32(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadUI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value uint16
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_ui16(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadUI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value uint8
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_ui8(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value int64
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_i64(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value int32
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_i32(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value int16
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_i16(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value int8
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_i8(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadBOOL(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var value bool
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Read(file, binary.LittleEndian, &value); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, value)
	outputs[1].Set_bool(prgrm, success)
}

// VIOLATION STRHOT-135: B, hands a path or text to Go or the OS, #412.
func opOsWriteStr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		value := inputs[1].Get_str(prgrm)
		len := len(value)
		if err := binary.Write(file, binary.LittleEndian, uint64(len)); err == nil {
			if err := binary.Write(file, binary.LittleEndian, []byte(value)); err == nil {
				success = true
			}
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteF64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_f64(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteF32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_f32(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteUI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_ui64(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteUI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_i32(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteUI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_ui16(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteUI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_ui8(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_i64(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_i32(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_i16(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_i8(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteBOOL(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if err := binary.Write(file, binary.LittleEndian, inputs[1].Get_bool(prgrm)); err == nil {
			success = true
		}
	}

	outputs[0].Set_bool(prgrm, success)
}

func getSlice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) (outputSlicePointer types.Pointer, outputSliceOffset types.Pointer, sizeofElement types.Pointer, count types.Pointer) {
	var inp1, out0 *ast.CXArgument

	var inpType, outType types.Code
	var inpIsSlice, outIsSlice bool
	var inpSize types.Pointer
	if inputs[1].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1388.
		inp1 = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[1].TypeSignature.Meta))

		inpType = inp1.Type
		inpIsSlice = (inp1.GetAssignmentElement(prgrm)).IsSlice()
		inpSize = (inp1.GetAssignmentElement(prgrm)).Size
	} else if inputs[1].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1389.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(inputs[1].TypeSignature.Meta)
		inpType = types.Code(sliceDetails.Meta)
		inpIsSlice = true
		inpSize = inpType.Size()
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	if outputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1390.
		out0 = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outputs[0].TypeSignature.Meta))

		outType = out0.Type
		outIsSlice = (out0.GetAssignmentElement(prgrm)).IsSlice()
	} else if outputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1391.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(outputs[0].TypeSignature.Meta)

		outType = types.Code(sliceDetails.Meta)
		outIsSlice = true
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	if inpType != outType || !inpIsSlice || !outIsSlice {
		panic(types.CX_RUNTIME_INVALID_ARGUMENT)
	}

	outputSlicePointer = outputs[0].Offset
	sizeofElement = inpSize
	// The count is a ui64. One past what the heap can hold reads nothing.
	count64 := inputs[2].Get_ui64(prgrm)
	if count64 > uint64(ast.SliceElementLimit(sizeofElement)) {
		outputSliceOffset = ast.GetSliceOffset(prgrm, inputs[1].FramePointer, inputs[1].TypeSignature)
		return
	}
	count = types.Pointer(count64)
	outputSliceOffset = ast.SliceResize(prgrm, outputs[0].FramePointer, outputs[0].TypeSignature, inputs[1].TypeSignature, count, sizeofElement)
	return
}

func opOsReadF64Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]float64, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_f64(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadF32Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]float32, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_f32(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadUI64Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]uint64, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_ui64(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadUI32Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]uint32, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_ui32(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadUI16Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]uint16, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_ui16(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadUI8Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]uint8, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_ui8(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadI64Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]int64, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_i64(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadI32Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]int32, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_i32(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadI16Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]int16, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_i16(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsReadI8Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	_, outputSliceOffset, sizeofElement, count := getSlice(prgrm, inputs, outputs)
	if count > 0 {
		if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
			values := make([]int8, count)
			if err := binary.Read(file, binary.LittleEndian, values); err == nil {
				success = true
				outputSliceData := ast.GetSliceData(prgrm, outputSliceOffset, sizeofElement)
				for i := types.Pointer(0); i < count; i++ {
					types.Write_i8(outputSliceData, i*sizeofElement, values[i])
				}
			}
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
	outputs[1].Set_bool(prgrm, success)
}

func opOsWriteF64Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_f64(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteF32Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_f32(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteUI64Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_f64(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteUI32Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_ui32(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteUI16Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_ui16(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteUI8Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_ui8(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteI64Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_i64(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteI32Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_i32(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteI16Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_i16(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

func opOsWriteI8Slice(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	success := false
	if file := ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if data := inputs[1].GetSlice_i8(prgrm); data != nil {
			if err := binary.Write(file, binary.LittleEndian, data); err == nil {
				success = true
			}
		}
	}
	outputs[0].Set_bool(prgrm, success)
}

// VIOLATION STRHOT-136: B, hands a path or text to Go or the OS, #412.
func opOsGetWorkingDirectory(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	wd, err := os.Getwd()
	if err == nil && !strings.HasSuffix(wd, string(os.PathSeparator)) {
		wd += string(os.PathSeparator)
	}
	outputs[0].Set_str(prgrm, wd)
}

// VIOLATION STRHOT-137: B, hands a path or text to Go or the OS, #412.
func opOsRun(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var runError int32 = OS_RUN_SUCCESS

	command := inputs[0].Get_str(prgrm)
	dir := inputs[3].Get_str(prgrm)
	args, err := splitCommand(command)
	if err != nil || len(args) == 0 {
		runError = OS_RUN_EMPTY_CMD
		if err != nil {
			runError = OS_RUN_START_FAILED
		}
		outputs[0].Set_i32(prgrm, runError)
		outputs[1].Set_i32(prgrm, 0)
		outputs[2].Set_str(prgrm, "")
		return
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	var cmdError int32 = 0

	timeoutMs := inputs[2].Get_i32(prgrm)
	timeout := time.Duration(math.MaxInt64)
	if timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	if err := cmd.Start(); err != nil {
		runError = OS_RUN_START_FAILED
	} else {
		done := make(chan error)
		go func() { done <- cmd.Wait() }()

		select {
		case <-time.After(timeout):
			cmd.Process.Kill()
			runError = OS_RUN_TIMEOUT
		case err := <-done:
			if err != nil {
				if exiterr, ok := err.(*exec.ExitError); ok {
					// from stackoverflow
					// The program has exited with an exit code != 0
					// This works on both Unix and Windows. Although package
					// syscall is generally platform dependent, WaitStatus is
					// defined for both Unix and Windows and in both cases has
					// an ExitStatus() method with the same signature.
					if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
						cmdError = int32(status.ExitStatus())
					}
				} else {
					runError = OS_RUN_WAIT_FAILED
				}
			}
		}
	}

	stdOutBytes := out.Bytes()
	maxSize := inputs[1].Get_i32(prgrm)
	if (maxSize > 0) && (len(stdOutBytes) > int(maxSize)) {
		stdOutBytes = stdOutBytes[0:maxSize]
	}

	outputs[0].Set_i32(prgrm, runError)
	outputs[1].Set_i32(prgrm, cmdError)
	outputs[2].Set_str(prgrm, string(stdOutBytes))
}

// splitCommand splits an os.Run command into words at spaces and tabs.
// Single or double quotes group a word. Backslashes are literal, so
// Windows paths need no escaping.
// VIOLATION STRHOT-138: B, hands a path or text to Go or the OS, #412.
func splitCommand(command string) ([]string, error) {
	var args []string
	var word strings.Builder
	inWord := false
	var quote rune
	for _, c := range command {
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inWord = true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if inWord {
				args = append(args, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", quote)
	}
	if inWord {
		args = append(args, word.String())
	}
	return args, nil
}
