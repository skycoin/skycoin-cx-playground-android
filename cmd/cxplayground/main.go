// Command cxplayground is the CX Playground app: a list of CX apps and a
// runner that gives the selected program a canvas, touch and key events and
// the soft keyboard. The UI is plain monospaced text at one size.
package main

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"io"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"

	"github.com/0magnet/gio/app"
	"github.com/0magnet/gio/f32"
	"github.com/0magnet/gio/font"
	"github.com/0magnet/gio/font/opentype"
	"github.com/0magnet/gio/io/event"
	"github.com/0magnet/gio/io/key"
	"github.com/0magnet/gio/io/pointer"
	"github.com/0magnet/gio/layout"
	"github.com/0magnet/gio/op"
	"github.com/0magnet/gio/op/clip"
	"github.com/0magnet/gio/op/paint"
	"github.com/0magnet/gio/text"
	"github.com/0magnet/gio/unit"
	"github.com/0magnet/gio/widget"
	"github.com/0magnet/gio/widget/material"

	"github.com/skycoin/skycoin-cx-playground-android/apprun"
	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

// The app bundle is built by `cxmanifest bundle` from apps/.
//
//go:embed bundle
var bundle embed.FS

// JetBrains Mono 2.304, SIL Open Font License 1.1 (font/OFL.txt).
//
//go:embed font/JetBrainsMono-Regular.ttf
var monoTTF []byte

// textSize is the one text size of the whole app, CX app.Text included.
const textSize = unit.Sp(14)

var (
	bg   = color.NRGBA{R: 0x27, G: 0x28, B: 0x22, A: 0xff}
	fg   = color.NRGBA{R: 0xf8, G: 0xf8, B: 0xf2, A: 0xff}
	dim  = color.NRGBA{R: 0x75, G: 0x71, B: 0x5e, A: 0xff}
	link = color.NRGBA{R: 0x66, G: 0xd9, B: 0xef, A: 0xff}
	mono = font.Font{Typeface: "JetBrains Mono"}
)

func main() {
	go func() {
		w := new(app.Window)
		w.Option(app.Title("CX Playground"))
		if err := run(w); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()
	app.Main()
}

type ui struct {
	w     *app.Window
	th    *material.Theme
	src   store.Source
	apps  []store.App
	errs  []error
	list  layout.List
	rows  []widget.Clickable
	back  widget.Clickable
	scrl  layout.List
	runID int

	// Set while a program is selected.
	cur      *store.App
	bench    *benchmarkScreen // set instead of host for a benchmark app
	host     *host
	out      *console
	done     chan struct{}
	result   string
	started  time.Time
	canvas   int // event tag
	focused  bool
	edit     []rune // what the IME believes the text field holds
	leaveReq bool

	// Test mode, see testmode.go. drawn is the program frame on screen.
	test        testSettings
	trace       *tracer
	drawn       programFrame
	tracedFrame int
	tracedTexts string
}

func run(w *app.Window) error {
	sub, err := fs.Sub(bundle, "bundle")
	if err != nil {
		return err
	}
	face, err := opentype.Parse(monoTTF)
	if err != nil {
		return err
	}
	u := &ui{w: w, src: store.FSSource{FS: sub}}
	u.th = material.NewTheme()
	u.th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection([]font.FontFace{{Font: mono, Face: face}}))
	u.th.Face, u.th.TextSize = mono.Typeface, textSize
	u.th.Palette.Fg, u.th.Palette.Bg = fg, bg
	u.list.Axis = layout.Vertical
	u.scrl.Axis = layout.Vertical
	u.apps, u.errs, err = store.List(u.src)
	if err != nil {
		return err
	}
	for _, a := range u.apps {
		log.Printf("cxplayground: app %s manifest verified: %s (%d files)", a.Hash, a.Manifest.Name, len(a.Manifest.Files))
	}
	for _, e := range u.errs {
		log.Printf("cxplayground: %v", e)
	}
	u.rows = make([]widget.Clickable, len(u.apps))
	if ts := loadTestSettings(); ts.enabled {
		u.test, u.trace = ts, &tracer{}
		u.trace.printf("test mode on, autostart %q", ts.autostart)
		u.autostart(ts.autostart)
	}

	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			paint.Fill(gtx.Ops, bg)
			if u.cur == nil {
				u.layoutList(gtx)
			} else {
				u.layoutRun(gtx)
			}
			e.Frame(gtx.Ops)
			if u.host != nil {
				u.host.frameShown()
				u.traceFrame()
			}
		}
	}
}

// text is one line of text, or wrapped lines when wrap is set.
func (u *ui) text(gtx layout.Context, c color.NRGBA, s string, wrap bool) layout.Dimensions {
	l := material.Label(u.th, textSize, s)
	l.Font, l.Color = mono, c
	if !wrap {
		l.MaxLines = 1
	}
	return l.Layout(gtx)
}

// line lays out spans side by side on one line, the last one truncated.
func (u *ui) line(gtx layout.Context, spans ...span) layout.Dimensions {
	ch := make([]layout.FlexChild, len(spans))
	for i, s := range spans {
		w := func(gtx layout.Context) layout.Dimensions { return u.text(gtx, s.c, s.s, false) }
		if i == len(spans)-1 {
			ch[i] = layout.Flexed(1, w)
		} else {
			ch[i] = layout.Rigid(w)
		}
	}
	return layout.Flex{}.Layout(gtx, ch...)
}

type span struct {
	c color.NRGBA
	s string
}

var (
	page = layout.Inset{Left: 12, Right: 12, Top: 8}
	row  = layout.Inset{Top: 10, Bottom: 10}
)

func (u *ui) layoutList(gtx layout.Context) {
	for i := range u.rows {
		if u.rows[i].Clicked(gtx) {
			u.start(&u.apps[i])
		}
	}
	page.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				s := fmt.Sprintf("  %d apps", len(u.apps))
				if len(u.errs) > 0 {
					s += fmt.Sprintf(", %d failed verification", len(u.errs))
				}
				return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.line(gtx, span{fg, "CX Playground"}, span{dim, s})
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return u.list.Layout(gtx, len(u.apps), func(gtx layout.Context, i int) layout.Dimensions {
					a := u.apps[i]
					return u.rows[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return u.line(gtx,
								span{dim, fmt.Sprintf("%2d  ", i+1)},
								span{fg, a.Manifest.Name + "  "},
								span{dim, a.Manifest.Description})
						})
					})
				})
			}),
		)
	})
}

// start loads, verifies and runs an app on its own goroutine.
func (u *ui) start(a *store.App) {
	u.cur, u.result, u.leaveReq, u.focused, u.edit = a, "", false, false, nil
	u.runID++
	if a.Manifest.Kind == "benchmark" {
		u.host, u.bench = nil, newBenchmarkScreen(u.src, *a, u.w.Invalidate, u.trace)
		u.trace.printf("start app=%q hash=%s", a.Manifest.Name, a.Hash)
		if u.test.benchmarkRuns > 0 && u.bench.files != nil {
			u.bench.runs = u.test.benchmarkRuns
			u.bench.start(u.w.Invalidate)
		}
		return
	}
	u.bench = nil
	u.host = newHost(u.w.Invalidate, u.trace)
	u.out = &console{invalidate: u.w.Invalidate, trace: u.trace}
	u.drawn, u.tracedFrame, u.tracedTexts = programFrame{}, 0, ""
	u.trace.printf("start app=%q hash=%s", a.Manifest.Name, a.Hash)
	u.done = make(chan struct{})
	u.started = time.Now()
	h, out, done := u.host, u.out, u.done
	go func() {
		defer close(done)
		defer u.w.Invalidate()
		files, err := store.Load(u.src, *a)
		if err != nil {
			fmt.Fprintf(out, "verification failed: %v\n", err)
			return
		}
		opts := cxrun.Options{Args: a.Manifest.Args, Stderr: out}
		if opts.Dir, err = writeDataFiles(*a, files); err != nil {
			fmt.Fprintf(out, "data files: %v\n", err)
			return
		}
		log.Printf("cxplayground: running %s (%s)", a.Manifest.Name, a.Hash)
		var stdout bytes.Buffer
		err = apprun.Run(a.Manifest, files, opts, h, io.MultiWriter(out, &stdout))
		if err != nil {
			fmt.Fprintf(out, "error: %v\n", err)
		}
		if u.trace != nil {
			saveTestStdout(u.trace, stdout.Bytes())
		}
		log.Printf("cxplayground: %s finished: %v", a.Manifest.Name, err)
		u.trace.printf("finished app=%q err=%v", a.Manifest.Name, err)
	}()
}

func (u *ui) finished() bool {
	select {
	case <-u.done:
		return true
	default:
		return false
	}
}

// leave asks the program to stop and returns to the list once it has.
func (u *ui) leave() {
	u.leaveReq = true
	if u.host != nil {
		u.host.closed.Store(true)
	}
}

func (u *ui) layoutRun(gtx layout.Context) {
	if u.back.Clicked(gtx) {
		u.leave()
	}
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameBack})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			u.leave()
		}
	}
	if u.bench != nil {
		u.layoutBenchmark(gtx)
		return
	}
	done := u.finished()
	if done && u.result == "" {
		u.result = fmt.Sprintf("finished in %d ms", time.Since(u.started).Milliseconds())
	}
	if done && u.leaveReq {
		gtx.Execute(key.SoftKeyboardCmd{Show: false})
		u.cur, u.host = nil, nil
		u.w.Invalidate()
		u.layoutList(gtx)
		return
	}
	f := u.host.takeFrame()
	cmds := f.cmds
	gfx := u.cur.Manifest.Kind == "gfx" && f.number > 0 && !done
	if gfx {
		u.drawn = f
	}

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.topBar(gtx, done)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if gfx || (u.cur.Manifest.Kind == "gfx" && !done) {
				return u.layoutCanvas(gtx, cmds)
			}
			return u.layoutConsole(gtx)
		}),
	)
}

// topBar shows the app's name and state, then its manifest hash and details,
// which the list leaves out.
func (u *ui) topBar(gtx layout.Context, done bool) layout.Dimensions {
	m := u.cur.Manifest
	kind := m.Kind
	if kind == "" {
		kind = "console"
	}
	var size int64
	for _, f := range m.Files {
		size += f.Size
	}
	state := "running"
	if done {
		state = u.result
	} else if u.leaveReq {
		state = "stopping when the program returns"
	}
	return page.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return u.back.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return u.text(gtx, link, "< apps  ", false)
							})
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return u.line(gtx, span{fg, m.Name})
						}),
					)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.text(gtx, dim, state, false)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return u.text(gtx, dim, "sha256 "+string(u.cur.Hash), true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				s := fmt.Sprintf("%s  %s  %d files  %d bytes", kind, m.Entry, len(m.Files), size)
				return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.text(gtx, dim, s, false)
				})
			}),
		)
	})
}

func (u *ui) layoutConsole(gtx layout.Context) layout.Dimensions {
	lines := strings.Split(strings.TrimRight(u.out.String(), "\n"), "\n")
	return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return u.scrl.Layout(gtx, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
			return u.text(gtx, fg, lines[i], true)
		})
	})
}

// layoutCanvas routes input to the program and draws its last frame.
func (u *ui) layoutCanvas(gtx layout.Context, cmds []cxrun.Cmd) layout.Dimensions {
	size := gtx.Constraints.Max
	u.host.cell.Store(u.measureCell(gtx))
	u.host.setSize(size)
	tag := &u.canvas
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, tag)

	// The canvas holds key focus so hardware keys and IME text reach the
	// program whether or not it opened the soft keyboard. The router drops
	// focus without a FocusEvent when the canvas misses a frame, so ask it.
	if f := gtx.Focused(tag); f != u.focused {
		u.focused = f
		u.trace.printf("input focused=%v", f)
	}
	if !u.focused {
		gtx.Execute(key.FocusCmd{Tag: tag})
	}
	if show, ok := u.host.takeKeyboard(); ok {
		gtx.Execute(key.SoftKeyboardCmd{Show: show})
	}
	for {
		ev, ok := gtx.Event(
			pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Drag | pointer.Release},
			key.FocusFilter{Target: tag},
			key.Filter{Focus: tag, Name: key.NameDeleteBackward},
			key.Filter{Focus: tag, Name: key.NameReturn},
			key.Filter{Focus: tag, Name: key.NameEnter},
		)
		if !ok {
			break
		}
		switch e := ev.(type) {
		case pointer.Event:
			k := map[pointer.Kind]int{pointer.Press: cxrun.EvTouchDown, pointer.Drag: cxrun.EvTouchMove, pointer.Release: cxrun.EvTouchUp}[e.Kind]
			if k != 0 {
				u.host.push(cxrun.Event{Kind: k, X: e.Position.X, Y: e.Position.Y})
			}
		case key.EditEvent:
			u.applyEdit(gtx, tag, e.Range, e.Text)
		case key.Event:
			if e.State != key.Press {
				break
			}
			switch e.Name {
			case key.NameDeleteBackward:
				n := len(u.edit)
				u.applyEdit(gtx, tag, key.Range{Start: max(n-1, 0), End: n}, "")
				if n == 0 {
					u.host.push(cxrun.Event{Kind: cxrun.EvKey, Code: cxrun.KeyBackspace})
				}
			case key.NameReturn, key.NameEnter:
				u.host.push(cxrun.Event{Kind: cxrun.EvKey, Code: cxrun.KeyEnter})
			}
		}
	}
	draw(gtx, u.th.Shaper, cmds)
	return layout.Dimensions{Size: size}
}

// measureCell returns the advance and line height of one character of the
// app font, from a 100 character line so the advance keeps its fraction.
func (u *ui) measureCell(gtx layout.Context) f32.Point {
	const n = 100
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(1<<20, 1<<20)}
	m := op.Record(gtx.Ops)
	dims, info := widget.Label{MaxLines: 1}.LayoutDetailed(g, u.th.Shaper, mono, textSize, strings.Repeat("M", n), op.CallOp{})
	m.Stop()
	if info.Truncated > 0 {
		return f32.Point{}
	}
	return f32.Pt(float32(dims.Size.X)/n, float32(dims.Size.Y))
}

func rgba(c cxrun.Cmd) color.NRGBA {
	b := func(f float32) uint8 {
		return uint8(min(max(f, 0), 1)*255 + 0.5)
	}
	return color.NRGBA{R: b(c.R), G: b(c.G), B: b(c.B), A: b(c.A)}
}

// draw renders a display list with Gio's GPU renderer.
func draw(gtx layout.Context, shaper *text.Shaper, cmds []cxrun.Cmd) {
	for _, c := range cmds {
		col := rgba(c)
		p := c.P
		switch c.Kind {
		case cxrun.CmdClear:
			paint.Fill(gtx.Ops, col)
		case cxrun.CmdRect:
			r := image.Rect(int(p[0]), int(p[1]), int(p[0]+p[2]), int(p[1]+p[3]))
			paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
		case cxrun.CmdCircle:
			r := image.Rect(int(p[0]-p[2]), int(p[1]-p[2]), int(p[0]+p[2]), int(p[1]+p[2]))
			paint.FillShape(gtx.Ops, col, clip.Ellipse(r).Op(gtx.Ops))
		case cxrun.CmdLine:
			var path clip.Path
			path.Begin(gtx.Ops)
			path.MoveTo(f32.Pt(p[0], p[1]))
			path.LineTo(f32.Pt(p[2], p[3]))
			paint.FillShape(gtx.Ops, col, clip.Stroke{Path: path.End(), Width: max(c.Width, 1)}.Op())
		case cxrun.CmdTri:
			var path clip.Path
			path.Begin(gtx.Ops)
			path.MoveTo(f32.Pt(p[0], p[1]))
			path.LineTo(f32.Pt(p[2], p[3]))
			path.LineTo(f32.Pt(p[4], p[5]))
			path.Close()
			paint.FillShape(gtx.Ops, col, clip.Outline{Path: path.End()}.Op())
		case cxrun.CmdText:
			t := op.Offset(image.Pt(int(p[0]), int(p[1]))).Push(gtx.Ops)
			m := op.Record(gtx.Ops)
			paint.ColorOp{Color: col}.Add(gtx.Ops)
			mat := m.Stop()
			g := gtx
			g.Constraints = layout.Constraints{Max: image.Pt(max(gtx.Constraints.Max.X-int(p[0]), 1), max(gtx.Constraints.Max.Y, 1))}
			widget.Label{}.Layout(g, shaper, mono, textSize, c.Text, mat)
			t.Pop()
		}
	}
}

// applyEdit replaces r in the mirrored text field with s, as the IME asked,
// and sends the program the difference as backspaces and runes. IMEs
// rewrite their composing text, so appending EditEvent.Text drops input.
func (u *ui) applyEdit(gtx layout.Context, tag event.Tag, r key.Range, s string) {
	old := u.edit
	start, end := min(r.Start, r.End), max(r.Start, r.End)
	start, end = min(max(start, 0), len(old)), min(max(end, 0), len(old))
	next := append(append(append([]rune{}, old[:start]...), []rune(s)...), old[end:]...)
	p := 0
	for p < len(old) && p < len(next) && old[p] == next[p] {
		p++
	}
	for range len(old) - p {
		u.host.push(cxrun.Event{Kind: cxrun.EvKey, Code: cxrun.KeyBackspace})
	}
	for _, c := range next[p:] {
		code := int32(c)
		if c == '\n' {
			code = cxrun.KeyEnter
		}
		u.host.push(cxrun.Event{Kind: cxrun.EvKey, Code: code})
	}
	u.edit = next
	n := len(next)
	gtx.Execute(key.SnippetCmd{Tag: tag, Snippet: key.Snippet{Range: key.Range{End: n}, Text: string(next)}})
	gtx.Execute(key.SelectionCmd{Tag: tag, Range: key.Range{Start: n, End: n}, Caret: key.Caret{}})
}

// autostart runs the app with the given manifest name, for the test driver.
func (u *ui) autostart(name string) {
	if name == "" {
		return
	}
	for i := range u.apps {
		if u.apps[i].Manifest.Name == name {
			u.start(&u.apps[i])
			return
		}
	}
	u.trace.printf("autostart: no app named %q", name)
}

// traceFrame reports a program frame once it is on screen, with the last
// event it reflects and, when they changed, the texts it shows.
func (u *ui) traceFrame() {
	f := u.drawn
	if u.trace == nil || f.number == u.tracedFrame {
		return
	}
	u.tracedFrame = f.number
	u.trace.printf("frame n=%d event=%d", f.number, f.afterEvent)
	if t := frameTexts(f.cmds); t != u.tracedTexts {
		u.tracedTexts = t
		u.trace.printf("text n=%d %s", f.number, t)
	}
}
