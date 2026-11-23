package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// testFolder is one folder under tests/: app/ holds the CX app (bundled into
// the APK by make bundle), input.txt the events, expect.txt the expected
// output, and expected/<name>.png optional reference screenshots.
type testFolder struct {
	dir     string
	name    string // folder name
	appName string // manifest name, used to autostart the app
	steps   []inputStep
	expect  []expectation
}

func loadTestFolder(dir string) (testFolder, error) {
	t := testFolder{dir: dir, name: filepath.Base(dir)}
	b, err := os.ReadFile(filepath.Join(dir, "app", "cxapp.json"))
	if err != nil {
		return t, err
	}
	var app struct{ Name string }
	if err := json.Unmarshal(b, &app); err != nil {
		return t, fmt.Errorf("%s: %w", dir, err)
	}
	t.appName = app.Name
	for file, parse := range map[string]func(*os.File) error{
		"input.txt":  func(f *os.File) (err error) { t.steps, err = parseSteps(f); return },
		"expect.txt": func(f *os.File) (err error) { t.expect, err = parseExpectations(f); return },
	} {
		f, err := os.Open(filepath.Join(dir, file))
		if err != nil {
			return t, err
		}
		err = parse(f)
		f.Close()
		if err != nil {
			return t, fmt.Errorf("%s/%s: %w", dir, file, err)
		}
	}
	return t, nil
}

// session holds what every test of a run shares.
type session struct {
	dev       *device
	appID     string
	results   string // results directory of this run
	tolerance uint32 // per channel, for screenshot comparison
	maxDiff   float64
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
	finish := func() {
		d.stopApp(ctx, s.appID)
		if err := table.timed("uninstall", func() error { return d.uninstall(ctx, s.appID) }); err != nil {
			fail("uninstall: %v", err)
		}
		table.add("total", time.Since(begin), 0, "")
		os.WriteFile(filepath.Join(out, "logcat.txt"), []byte(w.text()), 0o644)
	}

	if err := table.timed("install", func() error { return d.install(ctx, apk) }); err != nil {
		fail("install: %v", err)
		finish()
		return failures
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
	first, _, err := w.waitFor(firstCtx, 0, func(l logLine) bool { _, _, ok := l.frameTrace(); return ok })
	cancel()
	if err != nil {
		fail("no first frame of %q: %v", t.appName, err)
		finish()
		return failures
	}
	table.add("first frame (from start)", first.arrived.Sub(startBegin), 0, "")

	shots := map[string]string{}
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
			var ms int
			fmt.Sscan(step.args[0], &ms)
			time.Sleep(time.Duration(ms) * time.Millisecond)
		case step.injected():
			row, err := measureEvent(ctx, d, w, step)
			table.rows = append(table.rows, row)
			if err != nil {
				fail("%s: %v", step.line, err)
			}
		}
	}

	expectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
			f, err := s.compareShots(shots[e.args[0]], filepath.Join(t.dir, "expected", e.args[0]+".png"))
			if err != nil {
				fail("%s: %v", e.source, err)
			} else if f > s.maxDiff {
				fail("%s: %.2f%% of pixels differ, more than %.2f%%", e.source, f*100, s.maxDiff*100)
			}
		}
	}
	finish()
	return failures
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
		case "key":
			return len(events) >= 1
		}
		return len(events) > 0 && strings.Contains(events[len(events)-1].trace, "kind=touch-up")
	}
	// Resize events can arrive at any time and are not part of the step.
	wantKind := "kind=touch-"
	if step.verb == "text" || step.verb == "key" {
		wantKind = "kind=key "
	}
	var events []logLine
	var lastSeq int64
	scanned := from
	settle, cancel := context.WithTimeout(ctx, 3*time.Second)
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
