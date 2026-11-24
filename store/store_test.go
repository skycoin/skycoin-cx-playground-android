package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func testApps() map[string]map[string][]byte {
	return map[string]map[string][]byte{
		"t": {
			"cxapp.txt": []byte("[app]\nname = t\nkind = console\n"),
			"main.cx":   bytes.Repeat([]byte("package main\n"), 90),
			"lib/a.cx":  []byte("package main\n"),
			"empty.txt": nil,
		},
		"u": {
			"cxapp.txt": []byte("[app]\nname = u\nkind = console\n"),
			"main.cx":   []byte("package main\nfunc main() {}\n"),
		},
	}
}

func TestRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	rows, err := WriteBundle(dir, testApps(), nil)
	if err != nil {
		t.Fatal(err)
	}
	src := FSSource{FS: os.DirFS(dir)}
	apps, errs, err := List(src)
	if err != nil || len(errs) > 0 || len(apps) != 2 || string(apps[0].Hash) != rows[0].Address || apps[0].Manifest.Name != "t" {
		t.Fatal(apps, errs, err)
	}
	files, err := Load(src, apps[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(files["main.cx"], testApps()["t"]["main.cx"]) || len(files["empty.txt"]) != 0 {
		t.Fatal("content differs")
	}

	pack := filepath.Join(dir, PackingDir, rows[0].Address+".pack")
	b, _ := os.ReadFile(pack)
	b[700] ^= 1
	os.WriteFile(pack, b, 0o644)
	if _, err := Load(src, apps[0]); !errors.Is(err, programblocks.ErrBlockHash) {
		t.Fatalf("tampered block: %v", err)
	}
	if _, errs, _ := List(src); len(errs) != 1 {
		t.Fatalf("list of a tampered bundle: %v", errs)
	}
}

// TestSignaturesKept checks that a bundle of the same apps, written with the
// earlier bundle's signatures, has the same bytes, though signatures are not
// repeatable.
func TestSignaturesKept(t *testing.T) {
	first := filepath.Join(t.TempDir(), "first")
	if _, err := WriteBundle(first, testApps(), nil); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "second")
	if _, err := WriteBundle(second, testApps(), os.DirFS(first)); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(filepath.Join(first, ListFile))
	b, _ := os.ReadFile(filepath.Join(second, ListFile))
	if !bytes.Equal(a, b) {
		t.Fatal("the same apps gave another manifest list")
	}
	// In place, as make bundle writes it.
	if _, err := WriteBundle(first, testApps(), os.DirFS(first)); err != nil {
		t.Fatal(err)
	}
	if c, _ := os.ReadFile(filepath.Join(first, ListFile)); !bytes.Equal(a, c) {
		t.Fatal("writing in place changed the list")
	}
}

func TestBundledAppsVerify(t *testing.T) {
	src := FSSource{FS: os.DirFS("../cmd/cxplayground/bundle")}
	apps, errs, err := List(src)
	if err != nil || len(errs) > 0 || len(apps) == 0 {
		t.Fatal(apps, errs, err)
	}
	for _, a := range apps {
		if _, err := Load(src, a); err != nil {
			t.Errorf("%s: %v", a.Manifest.Name, err)
		}
	}
}
