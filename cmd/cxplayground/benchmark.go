package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/0magnet/gio/app"
	"github.com/0magnet/gio/layout"
	"github.com/0magnet/gio/widget"

	"github.com/skycoin/skycoin-cx-playground-android/benchsuite"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

// benchmarkUploadURL is where results are POSTed. Empty disables upload. Set
// it at build time: make apk BENCHMARK_UPLOAD_URL=https://host/runs
var benchmarkUploadURL string

// benchmarkScreen is the benchmark suite app: a run count, a start button,
// the progress and results, and upload.
type benchmarkScreen struct {
	files                      map[string][]byte
	runs                       int
	fewer, more, begin, upload widget.Clickable

	out     *console
	trace   *tracer
	started bool
	done    chan struct{}

	mu           sync.Mutex
	document     *benchsuite.ResultsDocument
	uploadStatus string
	uploading    bool
}

func newBenchmarkScreen(src store.Source, a store.App, invalidate func(), trace *tracer) *benchmarkScreen {
	s := &benchmarkScreen{runs: 5, out: &console{invalidate: invalidate, trace: trace}, trace: trace, done: make(chan struct{})}
	files, err := store.Load(src, a)
	if err != nil {
		fmt.Fprintf(s.out, "verification failed: %v\n", err)
		return s
	}
	s.files = files
	if suite, err := benchsuite.ParseSuiteDefinition(files[benchsuite.SuiteFile]); err == nil {
		s.runs = suite.Runs
	} else {
		fmt.Fprintf(s.out, "%v\n", err)
	}
	return s
}

func (s *benchmarkScreen) running() bool {
	if !s.started {
		return false
	}
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *benchmarkScreen) start(invalidate func()) {
	s.started = true
	go func() {
		defer close(s.done)
		defer invalidate()
		doc, err := benchsuite.RunSuite(s.files, s.runs, s.out)
		if err != nil {
			fmt.Fprintf(s.out, "error: %v\n", err)
			return
		}
		encoded, err := benchsuite.EncodeResultsDocument(doc)
		if err != nil {
			fmt.Fprintf(s.out, "error: %v\n", err)
			return
		}
		if path, err := saveBenchmarkResults(doc, encoded); err != nil {
			fmt.Fprintf(s.out, "not saved: %v\n", err)
		} else {
			s.trace.publishFile(filepath.ToSlash(filepath.Join("benchmark-results", filepath.Base(path))), encoded)
			fmt.Fprintf(s.out, "saved %s\n", path)
		}
		fmt.Fprintf(s.out, "\n%s", encoded)
		log.Printf("cxplayground: benchmark run %s finished", doc.RunID)
		s.mu.Lock()
		s.document = doc
		s.mu.Unlock()
	}()
}

// saveBenchmarkResults writes the document to benchmark-results/ in the
// app's files dir.
func saveBenchmarkResults(doc *benchsuite.ResultsDocument, encoded []byte) (string, error) {
	dir, err := app.DataDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "benchmark-results")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, doc.StartedAt.Format("20060102-150405")+"-"+doc.RunID+".txt")
	return path, os.WriteFile(path, encoded, 0o600)
}

func (s *benchmarkScreen) startUpload(invalidate func()) {
	s.mu.Lock()
	doc := s.document
	if doc == nil || s.uploading {
		s.mu.Unlock()
		return
	}
	s.uploading, s.uploadStatus = true, "uploading to "+benchmarkUploadURL
	s.mu.Unlock()
	go func() {
		reply, err := benchsuite.UploadResults(context.Background(), benchmarkUploadURL, doc)
		s.mu.Lock()
		s.uploading = false
		if err != nil {
			s.uploadStatus = "upload failed: " + err.Error()
		} else {
			s.uploadStatus = "uploaded: " + reply
		}
		s.mu.Unlock()
		invalidate()
	}()
}

// status is the state line of the top bar.
func (s *benchmarkScreen) status() string {
	switch {
	case s.running():
		return "running, back returns after the current program"
	case s.started:
		return "finished"
	}
	return "ready"
}

// layoutBenchmark draws the controls and the console under the top bar.
func (u *ui) layoutBenchmark(gtx layout.Context) {
	s := u.bench
	invalidate := u.w.Invalidate
	if u.leaveReq && !s.running() {
		u.cur, u.bench, u.host = nil, nil, nil
		u.w.Invalidate()
		u.layoutList(gtx)
		return
	}
	if !s.started && s.files != nil {
		if s.fewer.Clicked(gtx) && s.runs > 1 {
			s.runs--
		}
		if s.more.Clicked(gtx) && s.runs < 99 {
			s.runs++
		}
		if s.begin.Clicked(gtx) {
			s.start(invalidate)
		}
	}
	s.mu.Lock()
	finished, uploadStatus, uploading := s.document != nil, s.uploadStatus, s.uploading
	s.mu.Unlock()
	if finished && benchmarkUploadURL != "" && s.upload.Clicked(gtx) {
		s.startUpload(invalidate)
	}
	u.result = s.status()

	var controls []layout.Widget
	clickable := func(c *widget.Clickable, label string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return row.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return u.text(gtx, link, label, false)
				})
			})
		}
	}
	plain := func(col color.NRGBA, label string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return u.text(gtx, col, label, true) }
	}
	if !s.started && s.files != nil {
		controls = append(controls,
			plain(fg, fmt.Sprintf("runs per benchmark and executor: %d", s.runs)),
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(clickable(&s.fewer, "[ fewer ]  ")),
					layout.Rigid(clickable(&s.more, "[ more ]  ")),
					layout.Rigid(clickable(&s.begin, "[ start ]")),
				)
			},
			plain(dim, "runs count, fib, array, struct, memtest and primes on the old executor and the vm"),
		)
	}
	if finished {
		switch {
		case benchmarkUploadURL == "":
			controls = append(controls, plain(dim, "upload disabled: this build has no upload URL"))
		case uploading:
			controls = append(controls, plain(dim, uploadStatus))
		default:
			controls = append(controls, clickable(&s.upload, "[ upload results ]"))
			if uploadStatus != "" {
				controls = append(controls, plain(dim, uploadStatus))
			}
		}
	}
	lines := strings.Split(strings.TrimRight(s.out.String(), "\n"), "\n")

	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return u.topBar(gtx, !s.running())
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return u.scrl.Layout(gtx, len(controls)+len(lines), func(gtx layout.Context, i int) layout.Dimensions {
					if i < len(controls) {
						return controls[i](gtx)
					}
					return u.text(gtx, fg, lines[i-len(controls)], true)
				})
			})
		}),
	)
}
