package main

import (
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
)

// testSettings turn on the test mode that cmd/emulator-driver reads from
// logcat. On Android they come from system properties the driver sets with
// adb shell setprop, elsewhere from environment variables.
type testSettings struct {
	enabled   bool
	autostart string // manifest name of the app to run at launch
}

const (
	testProperty      = "debug.cxplayground.test"
	autostartProperty = "debug.cxplayground.autostart"
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
