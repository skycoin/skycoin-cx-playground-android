//go:build cxfx

package cxfx

import (
	"sync"
	"time"

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
	// APP_EVENT_COUNT is one past the last event type.
	APP_EVENT_COUNT
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

// cxfxAppCallbackSet records the CX function the events of eventType call and
// the window handle they pass. The compiler resolved the function, see #432.
func cxfxAppCallbackSet(prgrm *ast.CXProgram, eventType EventType, inputs []ast.CXValue) {
	cxfxState.CxfxAppCallbackTable[eventType] = CxfxCallback{
		CxfxCallbackWindow:   inputs[0].Get_i32(prgrm),
		CxfxCallbackFunction: ast.CXFunctionIndex(inputs[1].Get_i32(prgrm)),
		CxfxCallbackSet:      true,
	}
}

// cxfxAppCallbackCall calls the callback of eventType with the window handle
// and the count inputs after it that are already written to the scratch.
func cxfxAppCallbackCall(prgrm *ast.CXProgram, eventType EventType, count int) {
	callback := &cxfxState.CxfxAppCallbackTable[eventType]
	if !callback.CxfxCallbackSet {
		return
	}
	cxfxCallbackInputsI32(0, callback.CxfxCallbackWindow)
	execute.Callback(prgrm, prgrm.GetFunctionFromArray(callback.CxfxCallbackFunction), cxfxState.CxfxCallbackInputs[:count+1])
}

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

func PollEvents(prgrm *ast.CXProgram) {
	purgeEvents()
	eventCount := len(polled)
	for i := 0; i < eventCount; i++ {
		e := &polled[i]
		switch e.eventType {
		case APP_START, APP_STOP:
			cxfxAppCallbackCall(prgrm, e.eventType, 0)
		case APP_KEYBOARD:
			cxfxCallbackInputsI32(1, e.key)
			cxfxCallbackInputsI32(2, int32(e.scancode))
			cxfxCallbackInputsI32(3, int32(e.action))
			cxfxCallbackInputsI32(4, e.mods)
			cxfxAppCallbackCall(prgrm, e.eventType, 4)
		case APP_MOUSE:
			cxfxCallbackInputsI32(1, e.key)
			cxfxCallbackInputsI64(2, e.scancode)
			cxfxCallbackInputsI32(3, int32(e.action))
			cxfxCallbackInputsI32(4, e.mods)
			cxfxCallbackInputsF64(5, e.x)
			cxfxCallbackInputsF64(6, e.y)
			cxfxAppCallbackCall(prgrm, e.eventType, 6)
		case APP_FRAMEBUFFER_SIZE, APP_WINDOW_SIZE, APP_WINDOW_POSITION:
			cxfxCallbackInputsI32(1, int32(e.x)) // TODO : use float64 (deprecate int32)
			cxfxCallbackInputsI32(2, int32(e.y)) // TODO : use float64 (deprecate int32)
			cxfxAppCallbackCall(prgrm, e.eventType, 2)
		case APP_CURSOR_POS: // TODO : to deprecate
			cxfxCallbackInputsF64(1, e.x)
			cxfxCallbackInputsF64(2, e.y)
			cxfxAppCallbackCall(prgrm, e.eventType, 2)
		case APP_MOUSE_BUTTON: // TODO to deprecate
			cxfxCallbackInputsI32(1, e.key)
			cxfxCallbackInputsI32(2, int32(e.action))
			cxfxCallbackInputsI32(3, e.mods)
			cxfxAppCallbackCall(prgrm, e.eventType, 3)
		}
	}
	polled = polled[0:0]
}
