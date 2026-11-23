package cxrun

import (
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	actions "github.com/skycoin/skycoin-cx/t4-actions"
	opcodes "github.com/skycoin/skycoin-cx/t4-opcodes"
)

// Opcode numbers for the app package. skycoin-cx only registers the opcodes
// of its own table, so these sit above it and run on the old executor.
const opcodeBase = 4096

// Constant codes for the app package, clear of the ranges skycoin-cx uses.
const constBase = 0x7F000000

type handler = func(*cxast.CXProgram, []cxast.CXValue, []cxast.CXValue)

func register(p *cxast.CXProgram) {
	f32 := cxast.ConstCxArg_F32
	i32 := cxast.ConstCxArg_I32
	str := cxast.ConstCxArg_STR
	boolean := cxast.ConstCxArg_BOOL
	rgba := []*cxast.CXArgument{f32, f32, f32, f32}
	in := func(a ...*cxast.CXArgument) []*cxast.CXArgument { return a }
	cat := func(a, b []*cxast.CXArgument) []*cxast.CXArgument {
		return append(append([]*cxast.CXArgument{}, a...), b...)
	}
	code := opcodeBase
	fn := func(name string, h handler, inputs, outputs []*cxast.CXArgument) {
		registerHost(p, code, "app."+name, h, inputs, outputs)
		code++
	}

	fn("ScreenSize", opScreenSize, nil, in(i32, i32))
	fn("Clear", opClear, in(f32, f32, f32), nil)
	fn("Rect", opRect, cat(in(f32, f32, f32, f32), rgba), nil)
	fn("Circle", opCircle, cat(in(f32, f32, f32), rgba), nil)
	fn("Line", opLine, cat(in(f32, f32, f32, f32, f32), rgba), nil)
	fn("Tri", opTri, cat(in(f32, f32, f32, f32, f32, f32), rgba), nil)
	fn("Text", opText, cat(in(str, f32, f32, f32), rgba), nil)
	fn("Frame", opFrame, nil, nil)
	fn("PollEvent", opPollEvent, nil, in(i32, f32, f32, i32))
	fn("ShowKeyboard", opShowKeyboard, nil, nil)
	fn("HideKeyboard", opHideKeyboard, nil, nil)
	fn("ShouldClose", opShouldClose, nil, in(boolean))
	fn("Rune", opRune, in(i32), in(str))

	for i, c := range []struct {
		name string
		v    int32
	}{
		{"NONE", EvNone},
		{"TOUCH_DOWN", EvTouchDown},
		{"TOUCH_MOVE", EvTouchMove},
		{"TOUCH_UP", EvTouchUp},
		{"KEY", EvKey},
		{"RESIZE", EvResize},
		{"KEY_BACKSPACE", KeyBackspace},
		{"KEY_ENTER", KeyEnter},
	} {
		actions.AddConstI32(constBase+i, "app."+c.name, c.v)
	}
}

func color(p *cxast.CXProgram, in []cxast.CXValue, c *Cmd) {
	c.R, c.G, c.B, c.A = in[0].Get_f32(p), in[1].Get_f32(p), in[2].Get_f32(p), in[3].Get_f32(p)
}

func shape(kind, n int) handler {
	return func(p *cxast.CXProgram, in []cxast.CXValue, _ []cxast.CXValue) {
		c := Cmd{Kind: kind}
		for i := 0; i < n; i++ {
			c.P[i] = in[i].Get_f32(p)
		}
		color(p, in[n:], &c)
		if kind == CmdLine {
			c.Width = c.P[4]
		}
		cmds = append(cmds, c)
	}
}

var (
	opRect   = shape(CmdRect, 4)
	opCircle = shape(CmdCircle, 3)
	opLine   = shape(CmdLine, 5)
	opTri    = shape(CmdTri, 6)
)

func opScreenSize(p *cxast.CXProgram, _ []cxast.CXValue, out []cxast.CXValue) {
	w, h := host.Size()
	out[0].Set_i32(p, int32(w))
	out[1].Set_i32(p, int32(h))
}

func opClear(p *cxast.CXProgram, in []cxast.CXValue, _ []cxast.CXValue) {
	cmds = append(cmds[:0], Cmd{Kind: CmdClear, R: in[0].Get_f32(p), G: in[1].Get_f32(p), B: in[2].Get_f32(p), A: 1})
}

func opText(p *cxast.CXProgram, in []cxast.CXValue, _ []cxast.CXValue) {
	c := Cmd{Kind: CmdText, Text: in[0].Get_str(p)}
	c.P[0], c.P[1], c.Width = in[1].Get_f32(p), in[2].Get_f32(p), in[3].Get_f32(p)
	color(p, in[4:], &c)
	cmds = append(cmds, c)
}

func opFrame(*cxast.CXProgram, []cxast.CXValue, []cxast.CXValue) {
	frame := make([]Cmd, len(cmds))
	copy(frame, cmds)
	host.Frame(frame)
	cmds = cmds[:0]
}

func opPollEvent(p *cxast.CXProgram, _ []cxast.CXValue, out []cxast.CXValue) {
	e, _ := host.PollEvent()
	out[0].Set_i32(p, int32(e.Kind))
	out[1].Set_f32(p, e.X)
	out[2].Set_f32(p, e.Y)
	out[3].Set_i32(p, e.Code)
}

func opShowKeyboard(*cxast.CXProgram, []cxast.CXValue, []cxast.CXValue) { host.Keyboard(true) }
func opHideKeyboard(*cxast.CXProgram, []cxast.CXValue, []cxast.CXValue) { host.Keyboard(false) }

func opShouldClose(p *cxast.CXProgram, _ []cxast.CXValue, out []cxast.CXValue) {
	out[0].Set_bool(p, host.ShouldClose())
}

// opRune turns a key code into text. CX's sprintf has no %c.
func opRune(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	out[0].Set_str(p, string(rune(in[0].Get_i32(p))))
}

// registerHost is opcodes.RegisterOpCode without its check that code is a row
// of constants.OpcodeLifecycleTable.
func registerHost(p *cxast.CXProgram, code int, name string, h handler, inputs, outputs []*cxast.CXArgument) {
	if code >= len(cxast.OpcodeHandlers) {
		cxast.OpcodeHandlers = append(cxast.OpcodeHandlers, make([]cxast.OpcodeHandler, code+1-len(cxast.OpcodeHandlers))...)
	}
	cxast.OpcodeHandlers[code] = h
	cxast.OpNames[code] = name
	cxast.OpCodes[name] = code
	if inputs == nil {
		inputs = []*cxast.CXArgument{}
	}
	if outputs == nil {
		outputs = []*cxast.CXArgument{}
	}
	cxast.Natives[code] = opcodes.MakeNativeFunction(p, code, inputs, outputs)
}
