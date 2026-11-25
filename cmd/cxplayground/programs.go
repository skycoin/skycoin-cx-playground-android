package main

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/0magnet/gio/app"
	"github.com/0magnet/gio/layout"
	"github.com/0magnet/gio/widget"
	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programcheck"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

// programsKind marks the program loader: an app whose args are a server
// address and optional signer keys to trust. Programs from any valid signer
// are listed, and the user confirms one from a key that is not trusted
// before it runs (D27). Its screen lists the server's
// programs, and downloads, validates and runs them, see "Loader, lister and
// runner (#17)" in docs/ProgramDistribution.md.
const programsKind = "programs"

// refused colors the violations and verdict of a refused program.
var refused = color.NRGBA{R: 0xf9, G: 0x26, B: 0x72, A: 0xff}

// programsInRAM is the number of programs the phone keeps loaded.
const programsInRAM = 8

// programsScreen is the state of the program loader.
type programsScreen struct {
	app    store.App
	server string
	out    *console
	trace  *tracer

	refresh, download, validate, run, unload, clear widget.Clickable
	rowClicks                                       []widget.Clickable

	mu       sync.Mutex
	store    *programstore.Store
	memory   *programstore.Memory
	rows     []programstore.Row
	selected int // index in rows, -1 for none
	reports  map[string]*programcheck.Report
	busy     string // the operation in progress, "" when idle
}

func newProgramsScreen(a store.App, invalidate func(), trace *tracer) *programsScreen {
	s := &programsScreen{app: a, out: &console{invalidate: invalidate, trace: trace}, trace: trace, selected: -1, reports: map[string]*programcheck.Report{}}
	if len(a.Manifest.Args) < 1 {
		fmt.Fprintf(s.out, "a programs app takes the server and optional trusted signer keys as args\n")
		return s
	}
	s.server = a.Manifest.Args[0]
	s.operate("open", invalidate, func() error {
		dir, err := app.DataDir()
		if err != nil {
			return err
		}
		st, err := programstore.Open(filepath.Join(dir, "programs"))
		if err != nil {
			return err
		}
		for _, hex := range a.Manifest.Args[1:] {
			k, err := cipher.PubKeyFromHex(hex)
			if err != nil {
				return fmt.Errorf("signer %q: %v", hex, err)
			}
			if err := st.TrustSigner(k, "from the app args"); err != nil {
				return err
			}
		}
		rows, err := st.ReadIndex()
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.store, s.memory, s.rows = st, programstore.NewMemory(st, programsInRAM), rows
		s.mu.Unlock()
		fmt.Fprintf(s.out, "store %s, %d programs listed\n", st.Dir, len(rows))
		return s.sync()
	})
	return s
}

// operate runs op on its own goroutine unless another one runs.
func (s *programsScreen) operate(name string, invalidate func(), op func() error) {
	s.mu.Lock()
	if s.busy != "" {
		s.mu.Unlock()
		fmt.Fprintf(s.out, "busy: %s\n", s.busy)
		return
	}
	s.busy = name
	s.mu.Unlock()
	go func() {
		err := op()
		s.mu.Lock()
		s.busy = ""
		s.mu.Unlock()
		if err != nil {
			fmt.Fprintf(s.out, "%s failed: %v\n", name, err)
		}
		s.trace.printf("programs %s done err=%v", name, err)
		invalidate()
	}()
}

// client dials the server.
func (s *programsScreen) client() (*programblocks.Client, io.Closer, error) {
	conn, err := net.Dial("udp", s.server)
	if err != nil {
		return nil, nil, err
	}
	return programblocks.NewClient(conn), conn, nil
}

// sync fetches the server's manifest list into the store.
func (s *programsScreen) sync() error {
	c, conn, err := s.client()
	if err != nil {
		return err
	}
	defer conn.Close()
	start := time.Now()
	rows, err := s.store.Sync(c)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.rows = rows
	s.mu.Unlock()
	fmt.Fprintf(s.out, "synced %d programs from %s in %d ms\n", len(rows), s.server, time.Since(start).Milliseconds())
	return nil
}

// selectedRow returns the selected row and its address.
func (s *programsScreen) selectedRow() (programstore.Row, programblocks.Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.selected < 0 || s.selected >= len(s.rows) {
		return programstore.Row{}, programblocks.Hash{}, errors.New("select a program first")
	}
	r := s.rows[s.selected]
	addr, err := programblocks.HashFromHex(r.Address)
	return r, addr, err
}

func (s *programsScreen) reloadRows() {
	rows, err := s.store.ReadIndex()
	if err != nil {
		fmt.Fprintf(s.out, "index: %v\n", err)
		return
	}
	s.mu.Lock()
	s.rows = rows
	s.mu.Unlock()
}

func (s *programsScreen) downloadSelected() error {
	r, addr, err := s.selectedRow()
	if err != nil {
		return err
	}
	c, conn, err := s.client()
	if err != nil {
		return err
	}
	defer conn.Close()
	start := time.Now()
	files, err := s.store.Download(c, addr)
	s.reloadRows()
	if err != nil {
		return err
	}
	fmt.Fprintf(s.out, "downloaded %s: %d files, %d bytes, %d requests in %d ms, every block verified\n",
		r.Name, len(files), r.Length, c.Stats.RequestsSent, time.Since(start).Milliseconds())
	return nil
}

func (s *programsScreen) validateSelected() error {
	r, addr, err := s.selectedRow()
	if err != nil {
		return err
	}
	if r.State != programstore.StateCached {
		return fmt.Errorf("%s is %s, download it first", r.Name, r.State)
	}
	p, err := s.memory.Load(addr)
	if err != nil {
		return err
	}
	report, err := programcheck.CheckApp(p.App, p.Files, programcheck.PolicyFetched())
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.reports[r.Address] = report
	s.mu.Unlock()
	verdict := "runs"
	if !report.Allowed() {
		verdict = fmt.Sprintf("refused, %d sandbox violations", len(report.Violations))
	}
	fmt.Fprintf(s.out, "checked %s, loaded in RAM (%d bytes): %s\n", r.Name, p.Bytes, verdict)
	if signer, trusted, err := s.signer(addr); err == nil && trusted {
		fmt.Fprintf(s.out, "signer %s trusted\n", signer.Hex())
	} else if err == nil {
		fmt.Fprintf(s.out, "signer %s not trusted, run asks first\n", signer.Hex())
	}
	for _, line := range strings.Split(strings.TrimRight(report.Text(), "\n"), "\n") {
		s.trace.printf("report %s", line)
	}
	return nil
}

// signer returns the signer of the program at addr and whether it is in
// trusted.txt.
func (s *programsScreen) signer(addr programblocks.Hash) (cipher.PubKey, bool, error) {
	m, err := s.store.Manifest(addr)
	if err != nil {
		return cipher.PubKey{}, false, err
	}
	trusted, err := s.store.SignerTrusted(m.SignerPublicKey)
	return m.SignerPublicKey, trusted, err
}

// runnable returns the program to run when it is validated and allowed,
// and the function that prepares it. That function asks confirm about a
// signer that is not in trusted.txt before the program runs (D27).
func (s *programsScreen) runnable(confirm func(cipher.PubKey) bool) (*store.App, func(io.Writer) (*preparedProgram, error), error) {
	r, addr, err := s.selectedRow()
	if err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	report := s.reports[r.Address]
	s.mu.Unlock()
	if report == nil {
		return nil, nil, fmt.Errorf("validate %s first", r.Name)
	}
	if !report.Allowed() {
		return nil, nil, fmt.Errorf("refused to run %s: %d sandbox violations", r.Name, len(report.Violations))
	}
	p, err := s.memory.Load(addr)
	if err != nil {
		return nil, nil, err
	}
	signer, _, err := s.signer(addr)
	if err != nil {
		return nil, nil, err
	}
	trusted, err := s.store.TrustedSigners()
	if err != nil {
		return nil, nil, err
	}
	a := &store.App{Hash: manifest.Hash(r.Address), Manifest: p.App}
	prepared := &preparedProgram{app: p.App, files: p.Files, libraries: report.LibrariesToEnable(programcheck.PolicyFetched())}
	return a, func(out io.Writer) (*preparedProgram, error) {
		if err := reportSigner(out, signer, trusted, confirm); err != nil {
			return nil, err
		}
		return prepared, nil
	}, nil
}

func (s *programsScreen) unloadSelected(clearStore bool) error {
	r, addr, err := s.selectedRow()
	if err != nil {
		return err
	}
	s.memory.Unload(addr)
	s.mu.Lock()
	delete(s.reports, r.Address)
	s.mu.Unlock()
	if !clearStore {
		fmt.Fprintf(s.out, "unloaded %s from RAM\n", r.Name)
		return nil
	}
	if err := s.store.Clear(addr); err != nil {
		return err
	}
	s.reloadRows()
	fmt.Fprintf(s.out, "cleared %s from RAM and the store\n", r.Name)
	return nil
}

func (u *ui) layoutPrograms(gtx layout.Context) {
	s := u.programs
	invalidate := u.w.Invalidate
	s.mu.Lock()
	busy := s.busy
	rows := append([]programstore.Row(nil), s.rows...)
	selected := s.selected
	ready, memory := s.store != nil, s.memory
	s.mu.Unlock()
	if u.leaveReq && busy == "" {
		u.cur, u.programs = nil, nil
		u.w.Invalidate()
		u.layoutList(gtx)
		return
	}
	if len(s.rowClicks) != len(rows) {
		s.rowClicks = make([]widget.Clickable, len(rows))
	}
	for i := range s.rowClicks {
		if s.rowClicks[i].Clicked(gtx) {
			s.mu.Lock()
			s.selected, selected = i, i
			s.mu.Unlock()
			u.trace.printf("programs selected n=%d name=%q", rows[i].Number, rows[i].Name)
			fmt.Fprintf(s.out, "selected %d %s\n", rows[i].Number, rows[i].Name)
		}
	}
	// onSelected names an operation on the selected program, so the trace
	// line of its end is told apart from the same operation on another one.
	onSelected := func(op string) string {
		if selected >= 0 && selected < len(rows) {
			return op + " " + rows[selected].Name
		}
		return op
	}
	if s.refresh.Clicked(gtx) && ready {
		s.operate("refresh", invalidate, s.sync)
	}
	if s.download.Clicked(gtx) && ready {
		s.operate(onSelected("download"), invalidate, s.downloadSelected)
	}
	if s.validate.Clicked(gtx) && ready {
		s.operate(onSelected("validate"), invalidate, s.validateSelected)
	}
	if s.unload.Clicked(gtx) && ready {
		s.operate(onSelected("unload"), invalidate, func() error { return s.unloadSelected(false) })
	}
	if s.clear.Clicked(gtx) && ready {
		s.operate(onSelected("clear"), invalidate, func() error { return s.unloadSelected(true) })
	}
	if s.run.Clicked(gtx) && ready && busy == "" {
		a, prepare, err := s.runnable(u.confirmSigner)
		if err != nil {
			fmt.Fprintf(s.out, "run: %v\n", err)
			u.trace.printf("programs run refused err=%v", err)
		} else {
			u.launch(a, prepare)
			u.w.Invalidate()
			return
		}
	}

	u.result = "ready"
	if busy != "" {
		u.result = busy + "..."
	}
	clickable := func(c *widget.Clickable, label string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.text(gtx, link, label, false)
				})
			})
		})
	}
	var items []layout.Widget
	items = append(items, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx, clickable(&s.refresh, "[ refresh ]  "),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.text(gtx, dim, fmt.Sprintf("%s  %d programs", s.server, len(rows)), false)
				})
			}))
	})
	for i, r := range rows {
		i, r := i, r
		items = append(items, func(gtx layout.Context) layout.Dimensions {
			return s.rowClicks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					mark, col := "  ", fg
					if i == selected {
						mark, col = "> ", link
					}
					state := r.State
					if addr, err := programblocks.HashFromHex(r.Address); err == nil && memory != nil && memory.Held(addr) != nil {
						state += "+ram"
					}
					return u.line(gtx,
						span{dim, fmt.Sprintf("%s%2d  ", mark, r.Number)},
						span{col, fmt.Sprintf("%-15s ", r.Name)},
						span{dim, fmt.Sprintf("%-11s %5d", state, r.Length)})
				})
			})
		})
	}
	items = append(items, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx,
			clickable(&s.download, "[ download ] "),
			clickable(&s.validate, "[ validate ] "),
			clickable(&s.run, "[ run ]"))
	}, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx,
			clickable(&s.unload, "[ unload ] "),
			clickable(&s.clear, "[ clear ]"))
	})
	if selected >= 0 && selected < len(rows) {
		s.mu.Lock()
		report := s.reports[rows[selected].Address]
		s.mu.Unlock()
		if report != nil {
			items = append(items, func(gtx layout.Context) layout.Dimensions {
				return u.text(gtx, dim, "check of "+rows[selected].Name+" before it runs", false)
			})
			for _, line := range strings.Split(strings.TrimRight(report.Text(), "\n"), "\n") {
				col := fg
				if strings.HasPrefix(line, "  ") || line == "verdict refused" {
					col = refused
				}
				items = append(items, func(gtx layout.Context) layout.Dimensions { return u.text(gtx, col, line, true) })
			}
			items = append(items, func(gtx layout.Context) layout.Dimensions { return u.text(gtx, dim, " ", false) })
		}
	}
	lines := strings.Split(strings.TrimRight(s.out.String(), "\n"), "\n")

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.topBar(gtx, busy == "")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.scrl.Layout(gtx, len(items)+len(lines), func(gtx layout.Context, i int) layout.Dimensions {
					if i < len(items) {
						return items[i](gtx)
					}
					return u.text(gtx, fg, lines[i-len(items)], true)
				})
			})
		}),
	)
}
