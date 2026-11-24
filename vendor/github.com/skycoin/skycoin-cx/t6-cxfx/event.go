//go:build cxfx

package cxfx

import (
	"bytes"
	"sync"
	"time"

	"github.com/skycoin/skycoin-cx/t0-types"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-execute" // thin: only Callback; no depth gain
)

const (
	ACTION_RELEASE ActionType = 0 // TODO : break from glfw.Action
	ACTION_PRESS              = 1
	ACTION_REPEAT             = 2
	ACTION_MOVE               = 3
)

const (
	APP_NONE EventType = iota
	APP_START
	APP_STOP
	APP_KEYBOARD
	APP_MOUSE
	APP_FOCUS_ON
	APP_FOCUS_OFF
	APP_FRAMEBUFFER_SIZE
	APP_WINDOW_SIZE
	APP_WINDOW_POSITION
	APP_PAINT
	APP_CURSOR_POS   // TODO : to deprecate
	APP_MOUSE_BUTTON // TODO : to deprecate
)

type ActionType uint32
type EventType uint32
type Event struct {
	eventType EventType
	action    ActionType
	time      uint64
	x         float64
	y         float64
	key       int32
	scancode  int64
	mods      int32
}

type CXCallback struct {
	windowNameBytes []byte
	functionIndex   ast.CXFunctionIndex
	functionFound   bool
}

// init resolves the callback function when it is registered, so an event
// calls it by index without a name lookup, see #432.
// GCREACH: allocates on the heap, which adds to the GC pressure,
// see docs/GarbageCollector.md.
func (cb *CXCallback) init(prgrm *ast.CXProgram, inputs []ast.CXValue, packageNameGid stringpool.StringGid, packageFound bool) {
	windowName := bytes.Clone(ast.ProgramStringBytesRead(prgrm, &inputs[0]))
	var windowHeapPtr = ast.AllocateStr(prgrm, string(windowName))
	var windowNamePtr [types.POINTER_SIZE]byte
	types.Write_ptr(windowNamePtr[:], 0, windowHeapPtr)
	cb.windowNameBytes = windowNamePtr[:]
	cb.functionIndex, cb.functionFound = cxfxFunction(prgrm, packageNameGid, packageFound, ast.ProgramStringBytesRead(prgrm, &inputs[1]))
}

func (cb *CXCallback) Init(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cxAtomicOp := prgrm.CXAtomicOps[inputs[0].Expr.Index]

	pkg := prgrm.CXPackages[cxAtomicOp.Package]

	cb.init(prgrm, inputs, pkg.Name, true)
}

func (cb *CXCallback) InitEx(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	packageNameGid, packageFound := prgrm.CXProgramNameFindBytes(ast.ProgramStringBytesRead(prgrm, &inputs[2]))
	cb.init(prgrm, inputs, packageNameGid, packageFound)
}

func (cb *CXCallback) Call(prgrm *ast.CXProgram, inputs [][]byte) {
	if cb.functionFound {
		execute.Callback(prgrm, prgrm.GetFunctionFromArray(cb.functionIndex), inputs)
	}
}

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appKeyboardCallback CXCallback

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appMouseCallback CXCallback

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appFramebufferSizeCallback CXCallback

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appWindowSizeCallback CXCallback

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appWindowPosCallback CXCallback

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appStartCallback CXCallback

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appStopCallback CXCallback

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appCursorPositionCallback CXCallback // TODO : to deprecate
// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var appMouseButtonCallback CXCallback // TODO : to deprecate

// VIOLATION: state outside State. Owner: t6-cxfx; moves to EventMng in phase 1.
var eventCount int

// VIOLATION: state outside State. Owner: t6-cxfx; moves to EventMng in phase 1.
var events []Event

// VIOLATION: state outside State. Owner: t6-cxfx; moves to EventMng in phase 1.
var polled []Event
var polledCount int

// VIOLATION: state outside State. Owner: t6-cxfx; moves to EventMng in phase 1.
var mutex sync.Mutex

func PushEvent(eventType EventType) {
	mutex.Lock()
	defer mutex.Unlock()

	// VIOLATION: created outside manager; phase 1.
	event := Event{
		eventType,
		ACTION_RELEASE, // TODO : ACTION_NONE
		uint64(time.Now().UnixNano()),
		0.0,
		0.0,
		0,
		0,
		0,
	}
	if eventCount < len(events) {
		events[eventCount] = event
	} else {
		events = append(events, event)
	}
	eventCount++
}

func PushWindowSizeEvent(width float64, height float64) {
	index := eventCount
	PushEvent(APP_WINDOW_SIZE)
	events[index].x = width
	events[index].y = height
}

func PushWindowPositionEvent(x float64, y float64) {
	index := eventCount
	PushEvent(APP_WINDOW_POSITION)
	events[index].x = x
	events[index].y = y
}

func PushFramebufferSizeEvent(width float64, height float64) {
	index := eventCount
	PushEvent(APP_FRAMEBUFFER_SIZE)
	events[index].x = width
	events[index].y = height
}

func PushKeyboardEvent(action ActionType, key int32, scancode int64, mods int32) {
	index := eventCount
	PushEvent(APP_KEYBOARD)
	events[index].action = action
	events[index].key = key
	events[index].scancode = scancode
	events[index].mods = mods
}

func PushMouseEvent(eventType EventType, action ActionType, button int32, touch int64, mods int32, x float64, y float64) {
	index := eventCount
	PushEvent(eventType)
	events[index].action = action
	events[index].key = button
	events[index].scancode = touch
	events[index].mods = mods
	events[index].x = x
	events[index].y = y
}

func purgeEvents() {
	mutex.Lock()
	defer mutex.Unlock()

	if eventCount > 0 {
		polled = append(polled[0:0], events[0:eventCount]...)
		eventCount = 0
	}
}

func fromI32(value int32) []byte {
	var memory [4]byte
	types.Write_i32(memory[:], 0, value)
	return memory[:]
}

func fromI64(value int64) []byte {
	var memory [8]byte
	types.Write_i64(memory[:], 0, value)
	return memory[:]
}

func fromF64(value float64) []byte {
	var memory [8]byte
	types.Write_f64(memory[:], 0, value)
	return memory[:]
}

func PollEvents(prgrm *ast.CXProgram) {
	purgeEvents()
	eventCount := len(polled)
	for i := 0; i < eventCount; i++ {
		e := polled[i]
		switch e.eventType {
		case APP_START:
			var inputs [][]byte = make([][]byte, 1)
			inputs[0] = appStartCallback.windowNameBytes
			appStartCallback.Call(prgrm, inputs)
		case APP_STOP:
			var inputs [][]byte = make([][]byte, 1)
			inputs[0] = appStopCallback.windowNameBytes
			appStopCallback.Call(prgrm, inputs)
		case APP_KEYBOARD:
			var inputs [][]byte = make([][]byte, 5)
			inputs[0] = appKeyboardCallback.windowNameBytes
			inputs[1] = fromI32(e.key)
			inputs[2] = fromI32(int32(e.scancode))
			inputs[3] = fromI32(int32(e.action))
			inputs[4] = fromI32(e.mods)
			appKeyboardCallback.Call(prgrm, inputs)
		case APP_MOUSE:
			var inputs [][]byte = make([][]byte, 7)
			inputs[0] = appMouseCallback.windowNameBytes
			inputs[1] = fromI32(e.key)
			inputs[2] = fromI64(e.scancode)
			inputs[3] = fromI32(int32(e.action))
			inputs[4] = fromI32(e.mods)
			inputs[5] = fromF64(e.x)
			inputs[6] = fromF64(e.y)
			appMouseCallback.Call(prgrm, inputs)
		case APP_FRAMEBUFFER_SIZE:
			var inputs [][]byte = make([][]byte, 3)
			inputs[0] = appFramebufferSizeCallback.windowNameBytes
			inputs[1] = fromI32(int32(e.x)) // TODO : use float64 (deprecate int32)
			inputs[2] = fromI32(int32(e.y)) // TODO : use float64 (deprecate int32)
			appFramebufferSizeCallback.Call(prgrm, inputs)
		case APP_WINDOW_SIZE:
			var inputs [][]byte = make([][]byte, 3)
			inputs[0] = appWindowSizeCallback.windowNameBytes
			inputs[1] = fromI32(int32(e.x)) // TODO : use float64 (deprecate int32)
			inputs[2] = fromI32(int32(e.y)) // TODO : use float64 (deprecate int32)
			appWindowSizeCallback.Call(prgrm, inputs)
		case APP_WINDOW_POSITION:
			var inputs [][]byte = make([][]byte, 3)
			inputs[0] = appWindowPosCallback.windowNameBytes
			inputs[1] = fromI32(int32(e.x)) // TODO : use float64 (deprecate int32)
			inputs[2] = fromI32(int32(e.y)) // TODO : use float64 (deprecate int32)
			appWindowPosCallback.Call(prgrm, inputs)
		case APP_CURSOR_POS: // TODO : to deprecate
			var inputs [][]byte = make([][]byte, 3)
			inputs[0] = appCursorPositionCallback.windowNameBytes
			inputs[1] = fromF64(e.x)
			inputs[2] = fromF64(e.y)
			appCursorPositionCallback.Call(prgrm, inputs)
		case APP_MOUSE_BUTTON: // TODO to deprecate
			var inputs [][]byte = make([][]byte, 4)
			inputs[0] = appMouseButtonCallback.windowNameBytes
			inputs[1] = fromI32(e.key)
			inputs[2] = fromI32(int32(e.action))
			inputs[3] = fromI32(e.mods)
			appMouseButtonCallback.Call(prgrm, inputs)
		}
	}
	polled = polled[0:0]
}

// cxfxFunction finds the function a callback names, reading its name from
// program memory. The pool is frozen at run time, so nothing is interned.
func cxfxFunction(prgrm *ast.CXProgram, packageNameGid stringpool.StringGid, packageFound bool, functionName []byte) (ast.CXFunctionIndex, bool) {
	if !packageFound {
		return -1, false
	}
	functionNameGid, found := prgrm.CXProgramNameFindBytes(functionName)
	if !found {
		return -1, false
	}
	fn, err := prgrm.GetFunction(functionNameGid, packageNameGid)
	if err != nil {
		return -1, false
	}
	return ast.CXFunctionIndex(fn.Index), true
}
