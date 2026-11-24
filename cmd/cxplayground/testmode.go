package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/0magnet/gio/app"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

// testSettings turn on the test mode that cmd/emulator-driver reads from
// logcat. On Android they come from system properties the driver sets with
// adb shell setprop, elsewhere from environment variables.
type testSettings struct {
	enabled   bool
	autostart string // manifest name of the app to run at launch
	// benchmarkRuns, when above 0, starts the benchmark suite at once with
	// that many runs per benchmark and executor.
	benchmarkRuns int
}

const (
	testProperty      = "debug.cxplayground.test"
	autostartProperty = "debug.cxplayground.autostart"
	benchmarkProperty = "debug.cxplayground.benchmark.runs"
)

func loadTestSettings() testSettings {
	read := func(prop, env string) string {
		if v := os.Getenv(env); v != "" {
			return v
		}
		if runtime.GOOS != "android" {
			return ""
		}
		out, err := exec.Command("/system/bin/getprop", prop).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	s := testSettings{
		enabled:   read(testProperty, "CXPLAYGROUND_TEST") == "1",
		autostart: read(autostartProperty, "CXPLAYGROUND_AUTOSTART"),
	}
	s.benchmarkRuns, _ = strconv.Atoi(read(benchmarkProperty, "CXPLAYGROUND_BENCHMARK_RUNS"))
	if s.autostart != "" {
		s.enabled = true
	}
	return s
}

// tracer writes test mode lines. Every line starts with "cxtest " so the
// driver can tell them from other output. A nil tracer writes nothing.
type tracer struct{}

func (t *tracer) printf(format string, args ...any) {
	if t != nil {
		log.Printf("cxtest "+format, args...)
	}
}

// frameTexts joins the text commands of a display list, for the driver to
// check what a frame shows without reading pixels.
func frameTexts(cmds []cxrun.Cmd) string {
	var b strings.Builder
	for _, c := range cmds {
		if c.Kind != cxrun.CmdText {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(c.Text)
	}
	return b.String()
}

// testStdoutFile is where test mode saves the standard output of the last
// program, relative to the app's files dir.
const testStdoutFile = "cxtest/stdout.txt"

func saveTestStdout(t *tracer, b []byte) {
	dir, err := app.DataDir()
	if err == nil {
		p := filepath.Join(dir, filepath.FromSlash(testStdoutFile))
		if err = os.MkdirAll(filepath.Dir(p), 0o700); err == nil {
			err = os.WriteFile(p, b, 0o600)
		}
	}
	t.publishFile(testStdoutFile, b)
	t.printf("stdout saved bytes=%d err=%v", len(b), err)
}

// fileChunk is the number of bytes per logcat line of publishFile. Gio cuts
// log lines at 1024 bytes, and 512 bytes are 684 in base64.
const fileChunk = 512

// publishFile logs a file the app saved, path relative to its files dir, so
// the driver can read it from logcat. run-as cannot read the files of the
// release APK that gogio builds.
func (t *tracer) publishFile(path string, b []byte) {
	if t == nil {
		return
	}
	sum := sha256.Sum256(b)
	chunks := (len(b) + fileChunk - 1) / fileChunk
	t.printf("file path=%s bytes=%d chunks=%d sha256=%s", path, len(b), chunks, hex.EncodeToString(sum[:]))
	for i := 0; i < chunks; i++ {
		c := b[i*fileChunk : min((i+1)*fileChunk, len(b))]
		t.printf("file-chunk path=%s n=%d %s", path, i, base64.StdEncoding.EncodeToString(c))
	}
}

// writeDataFiles writes the app's files that are not CX sources to a fresh
// directory and returns it. The directory is the program's sandbox root, so
// it exists even when the app has no data files, for a program that writes
// its own.
func writeDataFiles(a store.App, files map[string][]byte) (string, error) {
	dir, err := app.DataDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "app-files", string(a.Hash))
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	_, err = store.WriteDataFiles(dir, files)
	return dir, err
}
