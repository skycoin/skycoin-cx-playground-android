package forthrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
)

// keyHost feeds keys and keeps the texts of the last frame.
type keyHost struct {
	keys   []int32
	frames int
	texts  []string
}

func (h *keyHost) Size() (int, int)             { return 720, 820 }
func (h *keyHost) CellSize() (float32, float32) { return 18, 40 }
func (h *keyHost) Keyboard(bool)                {}
func (h *keyHost) ShouldClose() bool            { return h.frames > 100 }
func (h *keyHost) Frame(cmds []cxrun.Cmd) {
	h.frames++
	h.texts = h.texts[:0]
	for _, c := range cmds {
		if c.Kind == cxrun.CmdText {
			h.texts = append(h.texts, c.Text)
		}
	}
}
func (h *keyHost) PollEvent() (cxrun.Event, bool) {
	if len(h.keys) == 0 {
		return cxrun.Event{}, false
	}
	k := h.keys[0]
	h.keys = h.keys[1:]
	return cxrun.Event{Kind: cxrun.EvKey, Code: k}, true
}

func runTestApp(t *testing.T, name string, keys string) (*keyHost, string) {
	src, err := os.ReadFile(filepath.Join("..", "tests", name, "app", "main.fs"))
	if err != nil {
		t.Fatal(err)
	}
	h := &keyHost{}
	for _, r := range keys {
		h.keys = append(h.keys, r)
	}
	var out bytes.Buffer
	if err := Run(string(src), h, &out); err != nil {
		t.Fatalf("%v, output %q", err, out.String())
	}
	return h, out.String()
}

func TestGridText(t *testing.T) {
	h, out := runTestApp(t, "forth-grid-text", "\n")
	want := []string{"  hello from cx-forth", "  6 x 7 = 42", "  grid 40 by 20", "bye"}
	if strings.Join(h.texts, "|") != strings.Join(want, "|") {
		t.Errorf("grid %q, want %q", h.texts, want)
	}
	if !strings.Contains(out, "hello from cx-forth") {
		t.Errorf("output %q", out)
	}
}

func TestKeyEcho(t *testing.T) {
	h, out := runTestApp(t, "forth-key-echo", "hi\nabc\n\n")
	want := "> hi\nlength 2 \n> abc\nlength 3 \n> done\n"
	if out != want {
		t.Errorf("output %q, want %q", out, want)
	}
	if len(h.texts) == 0 || h.texts[len(h.texts)-1] != "> done" {
		t.Errorf("grid %q", h.texts)
	}
}
