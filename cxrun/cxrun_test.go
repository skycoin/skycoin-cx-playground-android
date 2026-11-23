package cxrun

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeHost closes the program after a few frames and feeds it scripted events.
type fakeHost struct {
	frames int
	events []Event
	cmds   int
	kb     []bool
	texts  []string
}

func (h *fakeHost) Size() (int, int) { return 1080, 2000 }
func (h *fakeHost) Frame(c []Cmd) {
	h.frames++
	h.cmds += len(c)
	h.texts = h.texts[:0]
	for _, x := range c {
		if x.Kind == CmdText {
			h.texts = append(h.texts, x.Text)
		}
	}
}
func (h *fakeHost) Keyboard(show bool)           { h.kb = append(h.kb, show) }
func (h *fakeHost) CellSize() (float32, float32) { return 17, 37 }
func (h *fakeHost) ShouldClose() bool            { return h.frames >= 5 }
func (h *fakeHost) PollEvent() (Event, bool) {
	if len(h.events) == 0 {
		return Event{}, false
	}
	e := h.events[0]
	h.events = h.events[1:]
	return e, true
}

func readApp(t *testing.T, dir string) map[string][]byte {
	files := map[string][]byte{}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		files[e.Name()] = b
	}
	return files
}

// TestApps runs every bundled app with a main.cx, and every test folder app,
// twice in one process.
func TestApps(t *testing.T) {
	dirs, _ := filepath.Glob("../apps/*")
	testApps, _ := filepath.Glob("../tests/*/app")
	dirs = append(dirs, testApps...)
	if len(dirs) == 0 {
		t.Fatal("no apps")
	}
	for round := 0; round < 2; round++ {
		for _, d := range dirs {
			if _, err := os.Stat(filepath.Join(d, "main.cx")); err != nil {
				continue // the benchmark suite, run by tests/benchmark-hashes
			}
			name := filepath.Base(d)
			if name == "app" {
				name = filepath.Base(filepath.Dir(d))
			}
			t.Run(name, func(t *testing.T) {
				h := &fakeHost{events: []Event{
					{Kind: EvTouchDown, X: 300, Y: 1300}, {Kind: EvTouchMove, X: 320, Y: 1310},
					{Kind: EvTouchUp, X: 320, Y: 1310}, {Kind: EvTouchUp, X: 100, Y: 300}, {Kind: EvTouchUp, X: 700, Y: 300}, {Kind: EvTouchUp, X: 100, Y: 300}, {Kind: EvKey, Code: 'h'}, {Kind: EvKey, Code: 'i'},
					{Kind: EvKey, Code: KeyBackspace},
				}}
				var out bytes.Buffer
				err := Run(readApp(t, d), "main.cx", h, &out)
				t.Logf("frames=%d cmds=%d kb=%v\n%s", h.frames, h.cmds, h.kb, out.String())
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(out.String(), "error:") {
					t.Fatal("compiler errors")
				}
				all := strings.Join(h.texts, "|")
				switch filepath.Base(d) {
				case "keyboard":
					if fmt.Sprint(h.kb) != "[true false true]" || !strings.Contains(all, "|h_|") {
						t.Fatalf("keyboard: kb=%v texts=%q", h.kb, all)
					}
				case "events":
					if !strings.Contains(all, "typed: h|") || !strings.Contains(all, "screen 1080 x 2000 px") {
						t.Fatalf("events: texts=%q", all)
					}
				}
			})
		}
	}
}

// TestErrors checks that compile and runtime errors reach the output and that
// the next program still runs.
func TestErrors(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"package main\nfunc main() {\n\tundefinedFn()\n}\n", "main.cx:3:2"},
		{"package main\nfunc main() {\n\tvar a []i32\n\ti32.print(a[5])\n}\n", "CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE"},
		{"package main\nfunc main() {\n\ti32.print(1 + 2*3)\n}\n", "7\n"},
	} {
		var out bytes.Buffer
		err := Run(map[string][]byte{"main.cx": []byte(c.src)}, "main.cx", &fakeHost{}, &out)
		if !strings.Contains(out.String(), c.want) {
			t.Errorf("err=%v output %q does not contain %q", err, out.String(), c.want)
		}
	}
}

// TestExitArgsDir checks that os.Exit ends only the program, and that the
// program sees its arguments and its sandbox directory.
func TestExitArgsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("from the sandbox directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `package main
import "sys/os"
func main() {
	text, found := os.ReadAllText("data.txt")
	printf("%s %v %d %s\n", text, found, len(os.Args), os.Args[1])
	os.Exit(3)
	printf("not reached\n")
}
`
	var out bytes.Buffer
	opts := Options{Args: []string{"++mode=replay", "++tape=tape.txt"}, Dir: dir}
	err := RunWith(map[string][]byte{"main.cx": []byte(src)}, "main.cx", opts, &fakeHost{}, &out)
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 3 {
		t.Fatalf("err %v, want os.Exit(3)", err)
	}
	if want := "from the sandbox directory true 2 ++tape=tape.txt\n"; out.String() != want {
		t.Fatalf("output %q, want %q", out.String(), want)
	}
	out.Reset()
	if err := Run(map[string][]byte{"main.cx": []byte("package main\nfunc main() {\n\tprintf(\"next\\n\")\n}\n")}, "main.cx", &fakeHost{}, &out); err != nil || out.String() != "next\n" {
		t.Fatalf("next program: %v %q", err, out.String())
	}
}

// TestCellSize checks that app.CellSize returns the host's cell.
func TestCellSize(t *testing.T) {
	src := "package main\nimport \"sys/app\"\nfunc main() {\n\tvar w f32\n\tvar h f32\n\tw, h = app.CellSize()\n\tprintf(\"%d %d\\n\", f32.i32(w), f32.i32(h))\n}\n"
	var out bytes.Buffer
	if err := Run(map[string][]byte{"main.cx": []byte(src)}, "main.cx", &fakeHost{}, &out); err != nil || out.String() != "17 37\n" {
		t.Fatalf("%v %q", err, out.String())
	}
}
