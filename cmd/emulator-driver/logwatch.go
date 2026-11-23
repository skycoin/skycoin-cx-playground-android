package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// logLine is one logcat line of the app, in logcat's epoch format.
type logLine struct {
	raw     string
	device  time.Time // logcat timestamp on the device
	arrived time.Time // when the host read it
	trace   string    // the text after "cxtest ", or "" for other lines
}

func parseLogLine(raw string, arrived time.Time) logLine {
	l := logLine{raw: raw, arrived: arrived}
	f := strings.Fields(raw)
	if len(f) > 0 {
		if secs, err := strconv.ParseFloat(f[0], 64); err == nil {
			l.device = time.UnixMilli(int64(secs*1000 + 0.5))
		}
	}
	if i := strings.Index(raw, "cxtest "); i >= 0 {
		l.trace = raw[i+len("cxtest "):]
	}
	return l
}

// frameTrace parses "frame n=N event=S".
func (l logLine) frameTrace() (number int, event int64, ok bool) {
	var n int
	var e int64
	if _, err := fmt.Sscanf(l.trace, "frame n=%d event=%d", &n, &e); err != nil {
		return 0, 0, false
	}
	return n, e, true
}

// eventTrace parses "event seq=S ...".
func (l logLine) eventTrace() (seq int64, ok bool) {
	if !strings.HasPrefix(l.trace, "event seq=") {
		return 0, false
	}
	s, _, _ := strings.Cut(strings.TrimPrefix(l.trace, "event seq="), " ")
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

// logWatch streams the app's logcat and keeps every line, so steps can wait
// for a trace line and read when it arrived.
type logWatch struct {
	mu     sync.Mutex
	lines  []logLine
	update chan struct{} // closed and replaced on every new line
	done   chan struct{}
}

func watchLog(ctx context.Context, d *device, appID string) (*logWatch, error) {
	cmd := d.command(ctx, "logcat", "-v", "epoch", "-s", appID+":V")
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	w := &logWatch{update: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		w.read(out)
		cmd.Wait()
	}()
	return w, nil
}

func (w *logWatch) read(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		l := parseLogLine(sc.Text(), time.Now())
		w.mu.Lock()
		w.lines = append(w.lines, l)
		close(w.update)
		w.update = make(chan struct{})
		w.mu.Unlock()
	}
}

func (w *logWatch) snapshot() ([]logLine, chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]logLine(nil), w.lines...), w.update
}

// waitFor returns the first line from index from on that satisfies match.
func (w *logWatch) waitFor(ctx context.Context, from int, match func(logLine) bool) (logLine, int, error) {
	for {
		lines, update := w.snapshot()
		for i := from; i < len(lines); i++ {
			if match(lines[i]) {
				return lines[i], i, nil
			}
		}
		from = max(from, len(lines))
		select {
		case <-update:
		case <-w.done:
			return logLine{}, 0, io.ErrUnexpectedEOF
		case <-ctx.Done():
			return logLine{}, 0, ctx.Err()
		}
	}
}

func (w *logWatch) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.lines)
}

func (w *logWatch) traces() []string {
	lines, _ := w.snapshot()
	var out []string
	for _, l := range lines {
		if l.trace != "" {
			out = append(out, l.trace)
		}
	}
	return out
}

func (w *logWatch) text() string {
	lines, _ := w.snapshot()
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.raw)
		b.WriteByte('\n')
	}
	return b.String()
}
