// Package femtoclawreplay runs the bundled "Femtoclaw replay" app the way
// the playground app runs it, through store and cxrun, and checks its output
// against the host replay of microagents/femtoclaw.
package femtoclawreplay

import (
	"bytes"
	"os"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

const expectedOutput = "../../microagents/femtoclaw/tests/replay-determinism/expected-output.txt"

func TestBundledReplayMatchesHostReplay(t *testing.T) {
	want, err := os.ReadFile(expectedOutput)
	if err != nil {
		t.Fatal(err)
	}
	src := store.FSSource{FS: os.DirFS("../../cmd/cxplayground/bundle")}
	apps, errs, err := store.List(src)
	if err != nil || len(errs) > 0 {
		t.Fatal(err, errs)
	}
	var app *store.App
	for i := range apps {
		if apps[i].Manifest.Name == "Femtoclaw replay" {
			app = &apps[i]
		}
	}
	if app == nil {
		t.Fatal("no Femtoclaw replay app in the bundle")
	}
	files, err := store.Load(src, *app)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if wrote, err := store.WriteDataFiles(dir, files); err != nil || !wrote {
		t.Fatalf("data files: %v %v", wrote, err)
	}
	for _, replay := range []string{"first", "second"} {
		var out, stderr bytes.Buffer
		opts := cxrun.Options{Args: app.Manifest.Args, Dir: dir, Stderr: &stderr}
		if err := cxrun.RunWith(files, app.Manifest.Entry, opts, nil, &out); err != nil {
			t.Fatalf("%s replay: %v\n%s%s", replay, err, out.String(), stderr.String())
		}
		if !bytes.Equal(out.Bytes(), want) {
			t.Fatalf("%s replay: %d bytes differ from the %d bytes of %s\n%s", replay, out.Len(), len(want), expectedOutput, out.String())
		}
	}
}
