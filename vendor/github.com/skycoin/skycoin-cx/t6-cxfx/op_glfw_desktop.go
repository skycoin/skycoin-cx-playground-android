//go:build cxfx

package cxfx

import (
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/skycoin/skycoin-cx/t0-util"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// A window is an i32 handle into CxfxState.CxfxWindowTable, returned by
// glfw.CreateWindow, see #432.

func OpGlfwFullscreen(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	window := cxfxWindowGet(inputs[0].Get_i32(prgrm))
	monitor := glfw.GetPrimaryMonitor()
	mode := monitor.GetVideoMode()
	if inputs[1].Get_bool(prgrm) {
		window.SetMonitor(monitor, 0, 0, mode.Width, mode.Height, mode.RefreshRate)
	} else {
		window.SetMonitor(nil, int(inputs[2].Get_i32(prgrm)), int(inputs[3].Get_i32(prgrm)), int(inputs[4].Get_i32(prgrm)), int(inputs[5].Get_i32(prgrm)), mode.RefreshRate)
	}
	window.MakeContextCurrent()
}

func OpGlfwInit(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	glfw.Init()
	cxfxState.CxfxGlfwInitialized = true
}

func OpGlfwSwapBuffers(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm)).SwapBuffers()
}

func OpGlfwMakeContextCurrent(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm)).MakeContextCurrent()
}

func OpGlfwWindowHint(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	glfw.WindowHint(glfw.Hint(inputs[0].Get_i32(prgrm)), int(inputs[1].Get_i32(prgrm)))
}

func OpGlfwSetInputMode(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm)).SetInputMode(glfw.InputMode(inputs[1].Get_i32(prgrm)), int(inputs[2].Get_i32(prgrm)))
}

func OpGlfwGetCursorPos(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	x, y := cxfxWindowGet(inputs[0].Get_i32(prgrm)).GetCursorPos()
	outputs[0].Set_f64(prgrm, x)
	outputs[1].Set_f64(prgrm, y)
}

func OpGlfwGetKey(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(cxfxWindowGet(inputs[0].Get_i32(prgrm)).GetKey(glfw.Key(inputs[1].Get_i32(prgrm)))))
}

// VIOLATION STRHOT-170: B, hands the window title to GLFW, #432.
func OpGlfwCreateWindow(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	window, err := glfw.CreateWindow(int(inputs[0].Get_i32(prgrm)), int(inputs[1].Get_i32(prgrm)),
		util.ProgramTextStringView(ast.ProgramStringBytesRead(prgrm, &inputs[2])), nil, nil)
	if err != nil {
		panic(err)
	}
	outputs[0].Set_i32(prgrm, cxfxWindowAdd(window))
}

func OpGlfwGetWindowContentScale(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	xscale, yscale := cxfxWindowGet(inputs[0].Get_i32(prgrm)).GetContentScale()
	outputs[0].Set_f32(prgrm, xscale)
	outputs[1].Set_f32(prgrm, yscale)
}

func OpGlfwGetMonitorContentScale(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	xscale, yscale := glfw.GetPrimaryMonitor().GetContentScale()
	outputs[0].Set_f32(prgrm, xscale)
	outputs[1].Set_f32(prgrm, yscale)
}

func OpGlfwSetWindowPos(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm)).SetPos(int(inputs[1].Get_i32(prgrm)), int(inputs[2].Get_i32(prgrm)))
}

func OpGlfwShouldClose(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, cxfxWindowGet(inputs[0].Get_i32(prgrm)).ShouldClose())
}

func OpGlfwGetFramebufferSize(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	width, height := cxfxWindowGet(inputs[0].Get_i32(prgrm)).GetFramebufferSize()
	outputs[0].Set_i32(prgrm, int32(width))
	outputs[1].Set_i32(prgrm, int32(height))
}

func OpGlfwGetWindowPos(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	x, y := cxfxWindowGet(inputs[0].Get_i32(prgrm)).GetPos()
	outputs[0].Set_i32(prgrm, int32(x))
	outputs[1].Set_i32(prgrm, int32(y))
}

func OpGlfwGetWindowSize(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	width, height := cxfxWindowGet(inputs[0].Get_i32(prgrm)).GetSize()
	outputs[0].Set_i32(prgrm, int32(width))
	outputs[1].Set_i32(prgrm, int32(height))
}

func OpGlfwSwapInterval(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	glfw.SwapInterval(int(inputs[0].Get_i32(prgrm)))
}

func OpGlfwPollEvents(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	if cxfxState.CxfxGlfwInitialized {
		glfw.PollEvents()
	}
	PollEvents(prgrm)
}

func OpGlfwGetTime(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_f64(prgrm, glfw.GetTime())
}

func glfwSetKeyCallback(window *glfw.Window) {
	window.SetKeyCallback(func(w *glfw.Window, key glfw.Key, scancode int, action glfw.Action, mods glfw.ModifierKey) {
		PushKeyboardEvent(ActionType(action), int32(key), int64(scancode), int32(mods))
	})
}

func glfwSetCursorPosCallback(window *glfw.Window, eventType EventType) {
	window.SetCursorPosCallback(func(w *glfw.Window, xpos float64, ypos float64) {
		PushMouseEvent(eventType, ACTION_MOVE, 0, -1, 0, xpos, ypos)
	})
}

func glfwSetMouseButtonCallback(window *glfw.Window, eventType EventType) {
	window.SetMouseButtonCallback(func(w *glfw.Window, key glfw.MouseButton, action glfw.Action, mods glfw.ModifierKey) {
		x, y := w.GetCursorPos()
		PushMouseEvent(eventType, ActionType(action), int32(key), -1, int32(mods), x, y)
	})
}

func OpGlfwSetKeyCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) { // TODO : to deprecate
	glfwSetKeyCallback(cxfxWindowGet(inputs[0].Get_i32(prgrm)))
	cxfxAppCallbackSet(prgrm, APP_KEYBOARD, inputs)
}

func OpGlfwSetCursorPosCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) { // TODO : to deprecate
	glfwSetCursorPosCallback(cxfxWindowGet(inputs[0].Get_i32(prgrm)), APP_CURSOR_POS)
	cxfxAppCallbackSet(prgrm, APP_CURSOR_POS, inputs)
}

func OpGlfwSetMouseButtonCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) { // TODO : to deprecate
	glfwSetMouseButtonCallback(cxfxWindowGet(inputs[0].Get_i32(prgrm)), APP_MOUSE_BUTTON)
	cxfxAppCallbackSet(prgrm, APP_MOUSE_BUTTON, inputs)
}

func OpGlfwSetKeyboardCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	glfwSetKeyCallback(cxfxWindowGet(inputs[0].Get_i32(prgrm)))
	cxfxAppCallbackSet(prgrm, APP_KEYBOARD, inputs)
}

func OpGlfwSetMouseCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	window := cxfxWindowGet(inputs[0].Get_i32(prgrm))
	glfwSetCursorPosCallback(window, APP_MOUSE)
	glfwSetMouseButtonCallback(window, APP_MOUSE)
	cxfxAppCallbackSet(prgrm, APP_MOUSE, inputs)
}

func OpGlfwSetFramebufferSizeCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm)).SetFramebufferSizeCallback(func(w *glfw.Window, width int, height int) {
		PushFramebufferSizeEvent(float64(width), float64(height)) // TODO : to deprecate, use float64
	})
	cxfxAppCallbackSet(prgrm, APP_FRAMEBUFFER_SIZE, inputs)
}

func OpGlfwSetWindowSizeCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm)).SetSizeCallback(func(w *glfw.Window, width int, height int) {
		PushWindowSizeEvent(float64(width), float64(height)) // TODO : to deprecate, use float64
	})
	cxfxAppCallbackSet(prgrm, APP_WINDOW_SIZE, inputs)
}

func OpGlfwSetWindowPosCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm)).SetPosCallback(func(w *glfw.Window, x int, y int) {
		PushWindowPositionEvent(float64(x), float64(y)) // TODO to deprecate, use float64
	})
	cxfxAppCallbackSet(prgrm, APP_WINDOW_POSITION, inputs)
}

func OpGlfwSetStartCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm))
	cxfxAppCallbackSet(prgrm, APP_START, inputs)
	PushEvent(APP_START)
}

func OpGlfwSetStopCallback(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm))
	cxfxAppCallbackSet(prgrm, APP_STOP, inputs)
}

func OpGlfwSetShouldClose(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxfxWindowGet(inputs[0].Get_i32(prgrm)).SetShouldClose(inputs[1].Get_bool(prgrm))
}
