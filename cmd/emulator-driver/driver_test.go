package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
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

// TestFolders checks that every emulator test folder of the repository
// parses, and that the go test folders are told apart.
func TestFolders(t *testing.T) {
	dirs, _ := filepath.Glob("../../tests/*")
	emulator := 0
	for _, d := range dirs {
		tf, err := loadTestFolder(d)
		if errors.Is(err, errNotEmulatorTest) {
			if m, _ := filepath.Glob(filepath.Join(d, "*_test.go")); len(m) == 0 {
				t.Errorf("%s is neither an emulator test nor a go test", d)
			}
			continue
		}
		emulator++
		if err != nil {
			t.Fatal(err)
		}
		if tf.appName == "" || len(tf.steps) == 0 || len(tf.expect) == 0 {
			t.Fatalf("%s: incomplete test folder", d)
		}
	}
	if emulator == 0 {
		t.Fatal("no emulator test folders")
	}
}

func TestParseNewSteps(t *testing.T) {
	steps, err := parseSteps(strings.NewReader("rotate 1\nwait-for out done now\nkeyboard shown\npull cxtest/stdout.txt stdout.txt\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 4 || !steps[0].injected() || steps[1].args[0] != "out done now" || steps[3].args[1] != "stdout.txt" {
		t.Fatalf("%+v", steps)
	}
	if got := strings.Join(steps[0].inputCommand(), " "); got != "settings put system accelerometer_rotation 0 ; settings put system user_rotation 1" {
		t.Fatalf("rotate command %q", got)
	}
	for _, bad := range []string{"rotate 4", "rotate x", "keyboard open", "pull a", "wait-for"} {
		if _, err := parseSteps(strings.NewReader(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	expect, err := parseExpectations(strings.NewReader("screen-matches grid 0 100 720 300\nfile-equals out.txt ../x/expected.txt\nfile-contains doc.json \"status\": \"pass\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if r := region(expect[0]); r == nil || *r != image.Rect(0, 100, 720, 400) {
		t.Fatalf("region %v", r)
	}
	if expect[2].args[1] != `"status": "pass"` {
		t.Fatalf("file-contains %q", expect[2].args)
	}
	for _, bad := range []string{"screen-matches a 1 2 3", "screen-matches a 1 2 3 -4", "file-equals a", "file-contains a"} {
		if _, err := parseExpectations(strings.NewReader(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestParseSettings(t *testing.T) {
	var tf testFolder
	err := tf.parseSettings(strings.NewReader("app Benchmark suite\nready start app=\nsetprop debug.x 1\nbefore settings put secure a 1\nafter settings put secure a 0\ntimeout 90\ninput untimed\n"))
	if err != nil {
		t.Fatal(err)
	}
	if tf.appName != "Benchmark suite" || tf.ready != "start app=" || tf.props[0] != [2]string{"debug.x", "1"} ||
		tf.before[0] != "settings put secure a 1" || tf.after[0] != "settings put secure a 0" || tf.timeout != 90*time.Second || !tf.untimedInput {
		t.Fatalf("%+v", tf)
	}
	nodes := testFolder{dir: "tests/x"}
	if err := nodes.parseSettings(strings.NewReader("program-nodes 7010 3 publish a test seed\nprogram-node-type medium\n")); err != nil {
		t.Fatal(err)
	}
	if n := nodes.net; n.nodesPort != 7010 || n.nodesCount != 3 || n.nodesDir != "tests/x/publish" || n.nodesSeed != "a test seed" || n.nodesType != "medium" || !n.used() {
		t.Fatalf("program-nodes: %+v", n)
	}
	for _, bad := range []string{"fly 1", "setprop x", "timeout -1", "app", "input fast", "program-nodes 7010 3 dir", "program-nodes 7010 0 dir seed", "program-node-type huge", "program-node-type thin"} {
		if err := new(testFolder).parseSettings(strings.NewReader(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestPublishedFile(t *testing.T) {
	w := &logWatch{update: make(chan struct{}), done: make(chan struct{})}
	data := []byte("hello published file")
	sum := sha256.Sum256(data)
	trace := []string{
		"file path=other/a.txt bytes=1 chunks=1 sha256=00",
		fmt.Sprintf("file path=benchmark-results/x.json bytes=%d chunks=2 sha256=%x", len(data), sum),
		"file-chunk path=benchmark-results/x.json n=0 " + base64.StdEncoding.EncodeToString(data[:10]),
		"frame n=1 event=0",
		"file-chunk path=benchmark-results/x.json n=1 " + base64.StdEncoding.EncodeToString(data[10:]),
	}
	var raw strings.Builder
	for _, l := range trace {
		raw.WriteString("1763830000.125 1 1 I app: cxtest " + l + "\n")
	}
	w.read(strings.NewReader(raw.String()))
	close(w.done)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, err := w.publishedFile(ctx, "benchmark-results/*.json")
	if err != nil || string(b) != string(data) {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := w.publishedFile(ctx, "other/*.txt"); err == nil {
		t.Fatal("a file with a missing chunk was accepted")
	}
}
