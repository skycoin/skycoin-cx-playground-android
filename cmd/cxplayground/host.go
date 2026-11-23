package main

import (
	"image"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0magnet/gio/f32"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
)

// host is the cxrun.Host of the running program. The UI goroutine feeds it
// events and the canvas size and draws the frames it receives.
type host struct {
	invalidate func()
	trace      *tracer

	size   atomic.Value // image.Point
	cell   atomic.Value // f32.Point, see measureCell
	closed atomic.Bool

	mu        sync.Mutex
	events    []queuedEvent
	lastSeq   int64 // sequence number of the last queued event
	polledSeq int64 // sequence number of the last event the program polled
	frame     programFrame
	keyboard  *bool
	shown     chan struct{}
}

// queuedEvent is an event with its sequence number, counted from 1 per run.
type queuedEvent struct {
	event cxrun.Event
	seq   int64
}

// programFrame is a display list from the program. number counts frames from
// 1, and afterEvent is the last event the program polled before drawing it.
type programFrame struct {
	cmds       []cxrun.Cmd
	number     int
	afterEvent int64
}

func newHost(invalidate func(), trace *tracer) *host {
	h := &host{invalidate: invalidate, trace: trace, shown: make(chan struct{}, 1)}
	h.size.Store(image.Point{})
	h.cell.Store(f32.Point{})
	return h
}

func (h *host) Size() (int, int) {
	p := h.size.Load().(image.Point)
	return p.X, p.Y
}

func (h *host) CellSize() (float32, float32) {
	c := h.cell.Load().(f32.Point)
	return c.X, c.Y
}

func (h *host) setSize(p image.Point) {
	if old := h.size.Swap(p).(image.Point); old != p && old != (image.Point{}) {
		h.push(cxrun.Event{Kind: cxrun.EvResize, X: float32(p.X), Y: float32(p.Y)})
	}
}

// Frame hands the display list to the UI and waits until it was drawn, so
// the program runs at the display rate. The timeout keeps a program going
// while the window is not drawing.
func (h *host) Frame(cmds []cxrun.Cmd) {
	h.mu.Lock()
	h.frame = programFrame{cmds: cmds, number: h.frame.number + 1, afterEvent: h.polledSeq}
	h.mu.Unlock()
	select {
	case <-h.shown:
	default:
	}
	h.invalidate()
	select {
	case <-h.shown:
	case <-time.After(250 * time.Millisecond):
	}
}

// takeFrame returns the latest display list. Its number is 0 until the
// program has drawn.
func (h *host) takeFrame() programFrame {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.frame
}

func (h *host) frameShown() {
	select {
	case h.shown <- struct{}{}:
	default:
	}
}

var eventKindNames = map[int]string{
	cxrun.EvTouchDown: "touch-down",
	cxrun.EvTouchMove: "touch-move",
	cxrun.EvTouchUp:   "touch-up",
	cxrun.EvKey:       "key",
	cxrun.EvResize:    "resize",
}

func (h *host) push(e cxrun.Event) {
	h.mu.Lock()
	h.lastSeq++
	seq := h.lastSeq
	h.events = append(h.events, queuedEvent{event: e, seq: seq})
	h.mu.Unlock()
	h.trace.printf("event seq=%d kind=%s code=%d x=%.0f y=%.0f", seq, eventKindNames[e.Kind], e.Code, e.X, e.Y)
}

func (h *host) PollEvent() (cxrun.Event, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.events) == 0 {
		return cxrun.Event{}, false
	}
	q := h.events[0]
	h.events = h.events[1:]
	h.polledSeq = q.seq
	return q.event, true
}
func (h *host) Keyboard(show bool) {
	h.mu.Lock()
	h.keyboard = &show
	h.mu.Unlock()
	h.invalidate()
}

// takeKeyboard returns a pending keyboard request.
func (h *host) takeKeyboard() (show, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.keyboard == nil {
		return false, false
	}
	show = *h.keyboard
	h.keyboard = nil
	return show, true
}

func (h *host) ShouldClose() bool { return h.closed.Load() }

// console collects program output for display. In test mode it also traces
// each complete line.
type console struct {
	invalidate func()
	trace      *tracer
	mu         sync.Mutex
	b          strings.Builder
	traced     int // bytes of b already traced
}

func (c *console) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.b.Write(p)
	var lines []string
	if c.trace != nil {
		s := c.b.String()
		if i := strings.LastIndexByte(s, '\n'); i >= c.traced {
			lines = strings.Split(s[c.traced:i], "\n")
			c.traced = i + 1
		}
	}
	c.mu.Unlock()
	for _, l := range lines {
		c.trace.printf("out %s", l)
	}
	c.invalidate()
	return len(p), nil
}

func (c *console) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.b.String()
}
