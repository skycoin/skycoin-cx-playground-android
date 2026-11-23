package main

import (
	"image"
	"image/color"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseSteps(t *testing.T) {
	steps, err := parseSteps(strings.NewReader("# comment\ntext hello world\nkey ENTER\ntap 1 2\nswipe 1 2 3 4 300\nscreenshot end\nwait 50\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := [][]string{}
	for _, s := range steps {
		got = append(got, s.inputCommand())
	}
	want := [][]string{
		{"input", "text", "'hello%sworld'"},
		{"input", "keyevent", "KEYCODE_ENTER"},
		{"input", "tap", "1", "2"},
		{"input", "swipe", "1", "2", "3", "4", "300"},
		nil,
		nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for _, bad := range []string{"tap 1", "tap x y", "fly 1", "text", "swipe 1 2 3"} {
		if _, err := parseSteps(strings.NewReader(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestUnmatchedText(t *testing.T) {
	expect, err := parseExpectations(strings.NewReader("out a\nscreen-changed x y\nout c\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m := unmatchedText(expect, []string{"out a", "frame n=1 event=0", "out c"}); m != "" {
		t.Fatalf("unmatched %q", m)
	}
	if m := unmatchedText(expect, []string{"out c", "out a"}); m != "out c" {
		t.Fatalf("order not checked, got %q", m)
	}
}

func TestParseLogLine(t *testing.T) {
	l := parseLogLine("1763830000.125  4242  4260 I com.skycoin.cxplayground: cxtest frame n=3 event=7", time.Now())
	n, e, ok := l.frameTrace()
	if !ok || n != 3 || e != 7 {
		t.Fatalf("frame %d %d %v", n, e, ok)
	}
	if l.device.UnixMilli() != 1763830000125 {
		t.Fatalf("device time %v", l.device.UnixMilli())
	}
	l = parseLogLine("1763830000.200  4242  4260 I com.skycoin.cxplayground: cxtest event seq=12 kind=key code=104 x=0 y=0", time.Now())
	if seq, ok := l.eventTrace(); !ok || seq != 12 {
		t.Fatalf("event %d %v", seq, ok)
	}
	if _, ok := parseLogLine("1.0 1 1 I tag: hello", time.Now()).eventTrace(); ok {
		t.Fatal("plain line parsed as a trace")
	}
}

func TestTimingTable(t *testing.T) {
	var tt timingTable
	tt.add("install", 1500*time.Millisecond, 0, "")
	tt.add("event key ENTER", 40*time.Millisecond, 16*time.Millisecond, "1 events")
	want := "step               host ms  device ms  note\n" +
		"---------------    -------  ---------  ----\n" +
		"install               1500          -  \n" +
		"event key ENTER         40         16  1 events\n"
	if got := tt.String(); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestDifferingFraction(t *testing.T) {
	a := image.NewRGBA(image.Rect(0, 0, 10, 10))
	b := image.NewRGBA(image.Rect(0, 0, 10, 10))
	b.Set(0, 0, color.RGBA{R: 10, A: 0xff})
	b.Set(1, 0, color.RGBA{R: 200, A: 0xff})
	a.Set(0, 0, color.RGBA{A: 0xff})
	a.Set(1, 0, color.RGBA{A: 0xff})
	if f := differingFraction(a, b, 16); f != 0.01 {
		t.Fatalf("fraction %v", f)
	}
	if f := differingFraction(a, image.NewRGBA(image.Rect(0, 0, 5, 5)), 16); f != 1 {
		t.Fatalf("size mismatch fraction %v", f)
	}
}

// TestFolders checks that every test folder of the repository parses.
func TestFolders(t *testing.T) {
	dirs, _ := filepath.Glob("../../tests/*")
	if len(dirs) == 0 {
		t.Fatal("no test folders")
	}
	for _, d := range dirs {
		tf, err := loadTestFolder(d)
		if err != nil {
			t.Fatal(err)
		}
		if tf.appName == "" || len(tf.steps) == 0 || len(tf.expect) == 0 {
			t.Fatalf("%s: incomplete test folder", d)
		}
	}
}
