package cxrun

import (
	"fmt"

	types "github.com/skycoin/skycoin-cx/t0-types"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	actions "github.com/skycoin/skycoin-cx/t4-actions"
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
	fn("CellSize", opCellSize, nil, in(f32, f32))
	fn("NetDial", opNetDial, in(str, str), in(i32, str))
	fn("NetSend", opNetSend, in(i32, str), in(str))
	fn("NetRecv", opNetRecv, in(i32, i32), in(str, str))
	fn("NetClose", opNetClose, in(i32), nil)
	fn("ProgramFile", opProgramFile, in(str, cxast.Slice(types.UI8)), in(boolean))

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

func opCellSize(p *cxast.CXProgram, _ []cxast.CXValue, out []cxast.CXValue) {
	w, h := host.CellSize()
	out[0].Set_f32(p, w)
	out[1].Set_f32(p, h)
}

// ExitError is returned when the program calls os.Exit, which ends the
// program instead of the process that runs it.
type ExitError struct{ Code int32 }

func (e *ExitError) Error() string { return fmt.Sprintf("os.Exit(%d)", e.Code) }

// opExit stops the run loop with a panic that the loop records and
// runCompiledOn turns into an ExitError.
func opExit(p *cxast.CXProgram, in []cxast.CXValue, _ []cxast.CXValue) {
	panic(&ExitError{Code: in[0].Get_i32(p)})
}

// opRune turns a key code into text. CX's sprintf has no %c.
func opRune(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	out[0].Set_str(p, string(rune(in[0].Get_i32(p))))
}

// hostOpcode is one opcode of the app package, outside the opcode table of
// skycoin-cx.
type hostOpcode struct {
	hostHandler handler
	hostInputs  []*cxast.CXArgument
	hostOutputs []*cxast.CXArgument
}

// hostOpcodes holds the app opcodes by code minus opcodeBase.
var hostOpcodes []hostOpcode

// registerHost adds an app opcode. cxast.NativeFunctionHost gives the
// compiler its native and hostDispatch runs it.
func registerHost(p *cxast.CXProgram, code int, name string, h handler, inputs, outputs []*cxast.CXArgument) {
	if inputs == nil {
		inputs = []*cxast.CXArgument{}
	}
	if outputs == nil {
		outputs = []*cxast.CXArgument{}
	}
	for len(hostOpcodes) <= code-opcodeBase {
		hostOpcodes = append(hostOpcodes, hostOpcode{})
	}
	hostOpcodes[code-opcodeBase] = hostOpcode{hostHandler: h, hostInputs: inputs, hostOutputs: outputs}
	cxast.NativeNames.NativeNameIndexAdd(code, name)
}

// hostOpcodeOf returns the app opcode code names, or nil.
func hostOpcodeOf(code int) *hostOpcode {
	if code < opcodeBase || code-opcodeBase >= len(hostOpcodes) || hostOpcodes[code-opcodeBase].hostHandler == nil {
		return nil
	}
	return &hostOpcodes[code-opcodeBase]
}

// hostNativeFunction is cxast.NativeFunctionHost for the app opcodes.
func hostNativeFunction(p *cxast.CXProgram, code int) *cxast.CXNativeFunction {
	op := hostOpcodeOf(code)
	if op == nil {
		return nil
	}
	return &cxast.CXNativeFunction{AtomicOPCode: code, Inputs: op.hostInputs, Outputs: op.hostOutputs}
}

// hostDispatch returns the opcode switch of cx with os.Exit stopping the run
// loop and the app opcodes added.
func hostDispatch(core cxast.OpcodeDispatchFunction, exitCode int) cxast.OpcodeDispatchFunction {
	return func(p *cxast.CXProgram, code int, in []cxast.CXValue, out []cxast.CXValue) bool {
		if code == exitCode {
			opExit(p, in, out)
			return true
		}
		if op := hostOpcodeOf(code); op != nil {
			op.hostHandler(p, in, out)
			return true
		}
		return core(p, code, in, out)
	}
}

// programFile receives the files app.ProgramFile hands over during a run,
// Options.ProgramFile.
var programFile func(path string, data []byte) bool

// opProgramFile is app.ProgramFile(path str, data []ui8) (accepted bool): a
// loader program hands one verified file of the program the runner runs
// next. It is refused when the run has no Options.ProgramFile.
func opProgramFile(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	accepted := false
	if programFile != nil {
		data := append([]byte(nil), cxast.GetSliceData(p, in[1].Get_ptr(p), 1)...)
		accepted = programFile(in[0].Get_str(p), data)
	}
	out[0].Set_bool(p, accepted)
}
