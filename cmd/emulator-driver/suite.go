package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// session holds what every test of a run shares.
type session struct {
	dev       *device
	appID     string
	results   string // results directory of this run
	tolerance uint32 // per channel, for screenshot comparison
	maxDiff   float64
	// updateReferences writes the screen-matches captures to expected/
	// instead of comparing them.
	updateReferences bool
	rig              *rigContainer // nil when the device is not the rig
	repo             string
}

// buildAPK builds an x86_64 debug APK with the repository's make apk.
func buildAPK(ctx context.Context, repo, apk string) error {
	cmd := exec.CommandContext(ctx, "make", "-C", repo, "apk", "ARCH=amd64", "APK="+apk)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// runTest runs one test folder and returns its failures.
func (s *session) runTest(ctx context.Context, t testFolder, apk string, table *timingTable) []string {
	var failures []string
	fail := func(format string, args ...any) {
		failures = append(failures, fmt.Sprintf(format, args...))
	}
	out := filepath.Join(s.results, t.name)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return []string{err.Error()}
	}
	d := s.dev
	begin := time.Now()
	d.stopApp(ctx, s.appID)
	d.uninstall(ctx, s.appID)
	if err := d.clearLog(ctx); err != nil {
		return []string{err.Error()}
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	w, err := watchLog(watchCtx, d, s.appID)
	if err != nil {
		return []string{err.Error()}
	}
	var netStop func() (string, error)
	stopNet := func() string {
		if netStop == nil {
			return ""
		}
		path, err := netStop()
		netStop = nil
		if err != nil {
			fail("network settings: %v", err)
		}
		return path
	}
	finish := func() {
		stopNet()
		d.stopApp(ctx, s.appID)
		if err := table.timed("uninstall", func() error { return d.uninstall(ctx, s.appID) }); err != nil {
			fail("uninstall: %v", err)
		}
		for _, p := range t.props {
			d.shell(ctx, "setprop", p[0], "' '")
		}
		for _, c := range t.after {
			if _, err := d.shell(ctx, c); err != nil {
				fail("after %s: %v", c, err)
			}
		}
		d.shell(ctx, "settings", "put", "system", "user_rotation", "0")
		table.add("total", time.Since(begin), 0, "")
		os.WriteFile(filepath.Join(out, "logcat.txt"), []byte(w.text()), 0o644)
	}

	if err := table.timed("install", func() error { return d.install(ctx, apk) }); err != nil {
		fail("install: %v", err)
		finish()
		return failures
	}
	for _, p := range t.props {
		if _, err := d.shell(ctx, "setprop", p[0], shellQuote(p[1])); err != nil {
			fail("setprop %s: %v", p[0], err)
		}
	}
	for _, c := range t.before {
		if _, err := d.shell(ctx, c); err != nil {
			fail("before %s: %v", c, err)
		}
	}
	if t.net.used() {
		var err error
		if netStop, err = s.netSetup(ctx, t, out); err != nil {
			fail("network settings: %v", err)
			finish()
			return failures
		}
	}
	startBegin := time.Now()
	if err := table.timed("start (am start -W)", func() error { return d.startApp(ctx, s.appID, t.appName) }); err != nil {
		fail("start: %v", err)
		finish()
		return failures
	}
	firstCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	// The first frame on screen is not always the program's frame 1: frames
	// drawn before the window shows are replaced, not shown.
	ready, readyStep := func(l logLine) bool { _, _, ok := l.frameTrace(); return ok }, "first frame (from start)"
	if t.ready != "" {
		ready, readyStep = func(l logLine) bool { return strings.Contains(l.trace, t.ready) }, "ready line (from start)"
	}
	first, _, err := w.waitFor(firstCtx, 0, ready)
	cancel()
	if err != nil {
		fail("%s of %q not seen: %v", readyStep, t.appName, err)
		finish()
		return failures
	}
	table.add(readyStep, first.arrived.Sub(startBegin), 0, "")
	if err := table.timed("input ready (focus, touch mode off)", func() error { return s.waitInputReady(ctx, w, t) }); err != nil {
		fail("input not ready: %v", err)
		finish()
		return failures
	}

	shots := map[string]string{}
	pulled := map[string]string{}
	for i, step := range t.steps {
		switch {
		case step.verb == "screenshot":
			path := filepath.Join(out, fmt.Sprintf("%02d-%s.png", i+1, step.args[0]))
			err := table.timed("screenshot "+step.args[0], func() error {
				png, err := d.screenshot(ctx)
				if err != nil {
					return err
				}
				return os.WriteFile(path, png, 0o644)
			})
			if err != nil {
				fail("screenshot %s: %v", step.args[0], err)
			}
			shots[step.args[0]] = path
		case step.verb == "wait":
			ms, _ := strconv.Atoi(step.args[0])
			time.Sleep(time.Duration(ms) * time.Millisecond)
		case step.verb == "wait-for":
			waitCtx, cancel := context.WithTimeout(ctx, t.timeout)
			err := table.timed(step.line, func() error {
				_, _, err := w.waitFor(waitCtx, 0, func(l logLine) bool { return strings.Contains(l.trace, step.args[0]) })
				return err
			})
			cancel()
			if err != nil {
				fail("%s: %v", step.line, err)
			}
		case step.verb == "keyboard":
			if err := table.timed(step.line, func() error { return d.waitKeyboard(ctx, step.args[0] == "shown", 5*time.Second) }); err != nil {
				fail("%s: %v", step.line, err)
			}
		case step.verb == "pull":
			path := filepath.Join(out, step.args[1])
			err := table.timed(step.line, func() error {
				pullCtx, cancel := context.WithTimeout(ctx, t.timeout)
				defer cancel()
				b, err := w.publishedFile(pullCtx, step.args[0])
				if err != nil {
					return err
				}
				return os.WriteFile(path, b, 0o644)
			})
			if err != nil {
				fail("%s: %v", step.line, err)
			} else {
				pulled[step.args[1]] = path
			}
		case step.injected():
			row, err := measureEvent(ctx, d, w, step)
			table.rows = append(table.rows, row)
			if err != nil {
				fail("%s: %v", step.line, err)
			}
		}
	}

	if path := stopNet(); path != "" {
		pulled[t.net.capture] = path
	}
	expectCtx, cancel := context.WithTimeout(ctx, t.timeout)
	for {
		missing := unmatchedText(t.expect, w.traces())
		if missing == "" {
			break
		}
		_, update := w.snapshot()
		select {
		case <-update:
			continue
		case <-expectCtx.Done():
			fail("expected output not seen: %q", missing)
		}
		break
	}
	cancel()
	for _, e := range t.expect {
		switch e.verb {
		case "screen-changed":
			f, err := s.compareShots(shots[e.args[0]], shots[e.args[1]])
			if err != nil {
				fail("%s: %v", e.source, err)
			} else if f == 0 {
				fail("%s: the screenshots are identical", e.source)
			}
		case "screen-matches":
			if err := s.screenMatches(t, e, shots[e.args[0]], out); err != nil {
				fail("%s: %v", e.source, err)
			}
		case "file-equals":
			got, err := os.ReadFile(pulled[e.args[0]])
			want, err2 := os.ReadFile(filepath.Join(t.dir, filepath.FromSlash(e.args[1])))
			switch {
			case pulled[e.args[0]] == "":
				fail("%s: %s was not pulled", e.source, e.args[0])
			case err != nil || err2 != nil:
				fail("%s: %v %v", e.source, err, err2)
			case !bytes.Equal(got, want):
				fail("%s: %d bytes differ from the %d expected", e.source, len(got), len(want))
			}
		case "file-contains":
			got, err := os.ReadFile(pulled[e.args[0]])
			if pulled[e.args[0]] == "" || err != nil {
				fail("%s: %s was not pulled: %v", e.source, e.args[0], err)
			} else if !strings.Contains(string(got), e.args[1]) {
				fail("%s: not found", e.source)
			}
		}
	}
	finish()
	return failures
}

// region returns the x y w h region of a screen-matches line, or nil for
// the whole screen.
func region(e expectation) *image.Rectangle {
	if len(e.args) != 5 {
		return nil
	}
	var v [4]int
	for i := range v {
		v[i], _ = strconv.Atoi(e.args[i+1])
	}
	r := image.Rect(v[0], v[1], v[0]+v[2], v[1]+v[3])
	return &r
}

// screenMatches compares a screenshot, or its region, with the reference
// expected/<name>.png, which holds just the region. With updateReferences
// it writes the reference instead.
func (s *session) screenMatches(t testFolder, e expectation, shot, out string) error {
	if shot == "" {
		return errors.New("screenshot not taken")
	}
	img, err := readPNG(shot)
	if err != nil {
		return err
	}
	if r := region(e); r != nil {
		sub, ok := img.(interface {
			SubImage(image.Rectangle) image.Image
		})
		if !ok || !r.In(img.Bounds()) {
			return fmt.Errorf("region %v is outside the %v screenshot", *r, img.Bounds())
		}
		img = sub.SubImage(*r)
	}
	ref := filepath.Join(t.dir, "expected", e.args[0]+".png")
	if s.updateReferences {
		if err := os.MkdirAll(filepath.Dir(ref), 0o755); err != nil {
			return err
		}
		return writePNG(ref, img)
	}
	want, err := readPNG(ref)
	if err != nil {
		return err
	}
	f := differingFraction(img, want, s.tolerance)
	if f > s.maxDiff {
		writePNG(filepath.Join(out, e.args[0]+"-region.png"), img)
		return fmt.Errorf("%.2f%% of pixels differ, more than %.2f%%", f*100, s.maxDiff*100)
	}
	return nil
}

func writePNG(path string, img image.Image) error {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func (s *session) compareShots(a, b string) (float64, error) {
	if a == "" || b == "" {
		return 0, errors.New("screenshot not taken")
	}
	ia, err := readPNG(a)
	if err != nil {
		return 0, err
	}
	ib, err := readPNG(b)
	if err != nil {
		return 0, err
	}
	return differingFraction(ia, ib, s.tolerance), nil
}

// measureEvent injects one event and times it until the app shows the first
// frame drawn after the program polled the last event the injection caused.
// The host time runs from the adb command to the host reading that frame's
// trace line. The device time runs from the app queueing the first event to
// the frame, both by logcat timestamps.
func measureEvent(ctx context.Context, d *device, w *logWatch, step inputStep) (timingRow, error) {
	row := timingRow{step: "event " + step.line}
	from := w.count()
	begin := time.Now()
	if _, err := d.shell(ctx, step.inputCommand()...); err != nil {
		row.note = "FAILED"
		return row, err
	}
	complete := func(events []logLine) bool {
		switch step.verb {
		case "text":
			return len(events) >= utf8.RuneCountInString(step.args[0])
		case "key", "rotate":
			return len(events) >= 1
		}
		return len(events) > 0 && strings.Contains(events[len(events)-1].trace, "kind=touch-up")
	}
	// Resize events can arrive at any time and are not part of the step,
	// except for a rotation.
	wantKind := "kind=touch-"
	switch step.verb {
	case "text", "key":
		wantKind = "kind=key "
	case "rotate":
		wantKind = "kind=resize "
	}
	var events []logLine
	var lastSeq int64
	scanned := from
	settle, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for !complete(events) {
		l, i, err := w.waitFor(settle, scanned, func(l logLine) bool {
			_, ok := l.eventTrace()
			return ok && strings.Contains(l.trace, wantKind)
		})
		if err != nil {
			break
		}
		seq, _ := l.eventTrace()
		events, lastSeq, scanned = append(events, l), seq, i+1
	}
	if len(events) == 0 {
		row.note = "no event reached the app"
		return row, errors.New(row.note)
	}
	if !complete(events) {
		row.note = fmt.Sprintf("only %d events seen", len(events))
	}
	frameCtx, cancelFrame := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFrame()
	frame, _, err := w.waitFor(frameCtx, scanned, func(l logLine) bool {
		_, e, ok := l.frameTrace()
		return ok && e >= lastSeq
	})
	if err != nil {
		row.note = "no frame after the event"
		return row, errors.New(row.note)
	}
	row.host = frame.arrived.Sub(begin)
	row.device = frame.device.Sub(events[0].device)
	if row.device <= 0 {
		row.device = time.Microsecond // shown as 0, less than logcat's 1 ms resolution
	}
	if row.note == "" {
		row.note = fmt.Sprintf("%d events", len(events))
	}
	return row, nil
}

// waitInputReady waits until keys sent now reach the program: the app's
// window has focus in its own process, for an app that draws frames the
// latest "input focused" line of the canvas says true, and the display is
// out of touch mode.
func (s *session) waitInputReady(ctx context.Context, w *logWatch, t testFolder) error {
	if err := s.dev.waitWindowFocus(ctx, s.appID, t.timeout); err != nil {
		return err
	}
	if t.ready == "" {
		if err := waitCanvasFocus(ctx, w, t.timeout); err != nil {
			return err
		}
	}
	return s.dev.leaveTouchMode(ctx)
}

func waitCanvasFocus(ctx context.Context, w *logWatch, timeout time.Duration) error {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		lines, update := w.snapshot()
		if lastInputFocus(lines) {
			return nil
		}
		select {
		case <-update:
		case <-waitCtx.Done():
			return errors.New("the canvas has no key focus")
		}
	}
}
// lastInputFocus reports whether the latest "input focused" line says true.
func lastInputFocus(lines []logLine) bool {
	for i := len(lines) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(lines[i].trace, "input focused="); ok {
			return v == "true"
		}
	}
	return false
}
