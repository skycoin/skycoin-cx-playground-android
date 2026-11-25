//go:build cxfx

package cxfx

import (
	"unicode/utf8"

	"github.com/skycoin/gltext"

	"github.com/skycoin/skycoin-cx/t0-util"
	"github.com/skycoin/skycoin-cx/t3-ast"

	"github.com/skycoin/skycoin-cx/t5-cxos" // thin: only ValidFile; could move ValidFile to t0-util or t3-ast, max tier 11 to 10
)

// A font is an i32 handle into CxfxState.CxfxFontTable, returned by
// gltext.LoadTrueType, see #432. A file that does not load returns -1.
func loadTrueType(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue, fixedPipeline bool) {
	handle := int32(-1)
	if file := cxos.ValidFile(inputs[0].Get_i32(prgrm)); file != nil {
		if font, err := gltext.LoadTruetype(file,
			inputs[1].Get_i32(prgrm), rune(inputs[2].Get_i32(prgrm)), rune(inputs[3].Get_i32(prgrm)),
			gltext.Direction(inputs[4].Get_i32(prgrm)), fixedPipeline); err == nil {
			handle = cxfxFontAdd(font)
		}
	}
	outputs[0].Set_i32(prgrm, handle)
}

func OpGltextLoadTrueType(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	loadTrueType(prgrm, inputs, outputs, true)
}

func OpGltextLoadTrueTypeCore(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	loadTrueType(prgrm, inputs, outputs, false)
}

// VIOLATION STRHOT-185: B, hands the text to gltext as a view of program memory, #432.
func OpGltextPrintf(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	if err := cxfxFontGet(inputs[0].Get_i32(prgrm)).Printf(inputs[1].Get_f32(prgrm), inputs[2].Get_f32(prgrm), "%s", util.ProgramTextStringView(ast.ProgramStringBytesRead(prgrm, &inputs[3]))); err != nil {
		panic(err)
	}
}

// VIOLATION STRHOT-186: B, hands the text to gltext as a view of program memory, #432.
func OpGltextMetrics(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	width, height := cxfxFontGet(inputs[0].Get_i32(prgrm)).Metrics(util.ProgramTextStringView(ast.ProgramStringBytesRead(prgrm, &inputs[1])))
	outputs[0].Set_i32(prgrm, int32(width))
	outputs[1].Set_i32(prgrm, int32(height))
}

func OpGltextTexture(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(cxfxFontGet(inputs[0].Get_i32(prgrm)).Texture()))
}

func OpGltextNextGlyph(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	font := cxfxFontGet(inputs[0].Get_i32(prgrm))
	text := ast.ProgramStringBytesRead(prgrm, &inputs[1])
	index := int(inputs[2].Get_i32(prgrm))
	var runeValue rune = -1
	var width int = -1
	var x, y, w, h, advance int
	if index < len(text) {
		runeValue, width = utf8.DecodeRune(text[index:])
		g := font.Glyphs()[runeValue-font.Low()]
		x, y, w, h, advance = g.X, g.Y, g.Width, g.Height, g.Advance
	}
	outputs[0].Set_i32(prgrm, int32(runeValue-font.Low()))
	outputs[1].Set_i32(prgrm, int32(width))
	outputs[2].Set_i32(prgrm, int32(x))
	outputs[3].Set_i32(prgrm, int32(y))
	outputs[4].Set_i32(prgrm, int32(w))
	outputs[5].Set_i32(prgrm, int32(h))
	outputs[6].Set_i32(prgrm, int32(advance))
}

func OpGltextGlyphBounds(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	maxGlyphWidth, maxGlyphHeight := cxfxFontGet(inputs[0].Get_i32(prgrm)).GlyphBounds()
	outputs[0].Set_i32(prgrm, int32(maxGlyphWidth))
	outputs[1].Set_i32(prgrm, int32(maxGlyphHeight))
}

func OpGltextGlyphMetrics(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	width, height := cxfxFontGet(inputs[0].Get_i32(prgrm)).GlyphMetrics(uint32(inputs[1].Get_i32(prgrm)))
	outputs[0].Set_i32(prgrm, int32(width))
	outputs[1].Set_i32(prgrm, int32(height))
}

func OpGltextGlyphInfo(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	g := cxfxFontGet(inputs[0].Get_i32(prgrm)).Glyphs()[inputs[1].Get_i32(prgrm)]
	outputs[0].Set_i32(prgrm, int32(g.X))
	outputs[1].Set_i32(prgrm, int32(g.Y))
	outputs[2].Set_i32(prgrm, int32(g.Width))
	outputs[3].Set_i32(prgrm, int32(g.Height))
	outputs[4].Set_i32(prgrm, int32(g.Advance))
}
