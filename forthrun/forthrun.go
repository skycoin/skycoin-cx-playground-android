// Package forthrun runs Forth programs on the cx-forth VM inside the app. The
// VM's output goes to a terminal grid of the app font's cells, which the
// host draws, and host words give the program the grid and key input.
//
//	AT-XY      ( col row -- )   moves the cursor
//	PAGE       ( -- )           clears the grid, cursor to 0 0
//	GRID-SIZE  ( -- cols rows )
//	KEY        ( -- code )      waits for a key, 10 for ENTER, 8 for backspace
//	KEY?       ( -- flag )      whether a key is waiting
//	SHOW       ( -- )           draws the grid now
//
// The grid is drawn whenever the program waits for a key and when it ends.
package forthrun

import (
	"io"
	"strings"
	"unicode/utf8"

	"github.com/skycoin/skycoin-cx-forth/forth"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
)

// Grid size without a host, or before the host knows its canvas.
const (
	defaultCols = 40
	defaultRows = 20
)

// grid is the terminal grid the VM writes to.
type grid struct {
	cols, rows int
	cells      [][]rune
	x, y       int
	out        io.Writer // receives every byte the VM writes, too
}

func newGrid(cols, rows int, out io.Writer) *grid {
	g := &grid{cols: cols, rows: rows, out: out}
	g.page()
	return g
}

func (g *grid) page() {
	g.cells = make([][]rune, g.rows)
	for i := range g.cells {
		g.cells[i] = []rune(strings.Repeat(" ", g.cols))
	}
	g.x, g.y = 0, 0
}

func (g *grid) newline() {
	g.x = 0
	if g.y++; g.y == g.rows {
		g.cells = append(g.cells[1:], []rune(strings.Repeat(" ", g.cols)))
		g.y = g.rows - 1
	}
}

// Write puts text at the cursor. The VM's PAGE writes ESC [2J ESC [H, which
// clears the grid.
func (g *grid) Write(p []byte) (int, error) {
	if g.out != nil {
		g.out.Write(p)
	}
	s := string(p)
	for s != "" {
		if strings.HasPrefix(s, "\x1b[2J\x1b[H") {
			g.page()
			s = s[len("\x1b[2J\x1b[H"):]
			continue
		}
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		switch {
		case r == '\n':
			g.newline()
		case r == '\r':
			g.x = 0
		case r < ' ':
		default:
			if g.x == g.cols {
				g.newline()
			}
			g.cells[g.y][g.x] = r
			g.x++
		}
	}
	return len(p), nil
}

// lines returns the rows with their trailing spaces cut.
func (g *grid) lines() []string {
	l := make([]string, g.rows)
	for i, row := range g.cells {
		l[i] = strings.TrimRight(string(row), " ")
	}
	return l
}

// runner holds a run's VM, grid and host.
type runner struct {
	vm      *forth.VM
	g       *grid
	h       cxrun.Host
	cell    [2]float32
	pending []int32 // keys KEY? polled, for KEY
}

// Run interprets src with the grid words and output going to the grid and
// to out. h may be nil, then there is no input and nothing is drawn.
func Run(src string, h cxrun.Host, out io.Writer) error {
	r := &runner{h: h}
	cols, rows := defaultCols, defaultRows
	if h != nil {
		// The canvas size is known once the host has laid out a frame.
		for i := 0; i < 20; i++ {
			if w, ht := h.Size(); w > 0 && ht > 0 {
				break
			}
			h.Frame([]cxrun.Cmd{{Kind: cxrun.CmdClear, R: 0.06, G: 0.07, B: 0.08, A: 1}})
		}
		w, ht := h.Size()
		cw, ch := h.CellSize()
		if w > 0 && ht > 0 && cw > 0 && ch > 0 {
			cols, rows = int(float32(w)/cw), int(float32(ht)/ch)
		}
		r.cell = [2]float32{cw, ch}
	}
	r.g = newGrid(cols, rows, out)
	r.vm = forth.New(r.g)
	r.define()
	err := r.vm.Interpret(src)
	r.show()
	return err
}

func (r *runner) define() {
	vm := r.vm
	vm.Define("AT-XY", func(vm *forth.VM) {
		row, col := int(vm.PopInt()), int(vm.PopInt())
		r.g.x, r.g.y = min(max(col, 0), r.g.cols), min(max(row, 0), r.g.rows-1)
	})
	vm.Define("GRID-SIZE", func(vm *forth.VM) {
		vm.Push(forth.Int(int64(r.g.cols)))
		vm.Push(forth.Int(int64(r.g.rows)))
	})
	vm.Define("SHOW", func(*forth.VM) { r.show() })
	vm.Define("KEY?", func(vm *forth.VM) {
		_, ok := r.peekKey()
		vm.Push(forth.Bool(ok))
	})
	vm.Define("KEY", func(vm *forth.VM) {
		for {
			if code, ok := r.nextKey(); ok {
				vm.Push(forth.Int(int64(code)))
				return
			}
			if r.h == nil || r.h.ShouldClose() {
				vm.Fail("KEY: no more input")
				return
			}
			r.show()
		}
	})
}

var noKey = int32(-1)

// peekKey reports whether a key is waiting, keeping it for KEY.
func (r *runner) peekKey() (int32, bool) {
	if len(r.pending) > 0 {
		return r.pending[0], true
	}
	if r.h == nil {
		return noKey, false
	}
	code, ok := r.nextKey()
	if ok {
		r.pending = append(r.pending, code)
	}
	return code, ok
}

// nextKey returns the next key, skipping other events.
func (r *runner) nextKey() (int32, bool) {
	if len(r.pending) > 0 {
		c := r.pending[0]
		r.pending = r.pending[1:]
		return c, true
	}
	if r.h == nil {
		return noKey, false
	}
	for {
		e, ok := r.h.PollEvent()
		if !ok || e.Kind == cxrun.EvNone {
			return noKey, false
		}
		if e.Kind == cxrun.EvKey {
			return e.Code, true
		}
	}
}

// show draws the grid as one text command per row.
func (r *runner) show() {
	if r.h == nil {
		return
	}
	cmds := []cxrun.Cmd{{Kind: cxrun.CmdClear, R: 0.06, G: 0.07, B: 0.08, A: 1}}
	for i, l := range r.g.lines() {
		if l == "" {
			continue
		}
		c := cxrun.Cmd{Kind: cxrun.CmdText, Text: l, R: 0.85, G: 0.95, B: 0.85, A: 1}
		c.P[1] = float32(i) * r.cell[1]
		cmds = append(cmds, c)
	}
	r.h.Frame(cmds)
}
