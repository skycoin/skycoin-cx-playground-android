// Package cxformssame runs the bundled apps of each program form the way the
// playground app runs them, through store and apprun, and checks that the CX
// source, the serialized CX program and the cx-forth translation of one
// program print the same bytes.
package cxformssame

import (
	"bytes"
	"os"
	"testing"

	"github.com/skycoin/skycoin-cx-forth/cxir"

	"github.com/skycoin/skycoin-cx-playground-android/apprun"
	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

var programs = []struct {
	expected string   // the output file the emulator tests compare with
	apps     []string // manifest names, one per form
}{
	{"../cx-source-fib15/expected/stdout.txt", []string{"CX source: fib 0 to 14", "CX IR: fib 0 to 14", "cx-forth: fib 0 to 14"}},
	{"../cx-source-primes/expected/stdout.txt", []string{"CX source: primes", "CX IR: primes"}},
}

func TestFormsPrintTheSame(t *testing.T) {
	src := store.FSSource{FS: os.DirFS("../../cmd/cxplayground/bundle")}
	apps, errs, err := store.List(src)
	if err != nil || len(errs) > 0 {
		t.Fatal(err, errs)
	}
	byName := map[string]store.App{}
	for _, a := range apps {
		byName[a.Manifest.Name] = a
	}
	for _, p := range programs {
		want, err := os.ReadFile(p.expected)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range p.apps {
			t.Run(name, func(t *testing.T) {
				a, ok := byName[name]
				if !ok {
					t.Fatal("not in the bundle, run make bundle")
				}
				files, err := store.Load(src, a)
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				if err := apprun.Run(a.Manifest, files, cxrun.Options{}, nil, &out); err != nil {
					t.Fatalf("%v\n%s", err, out.Bytes())
				}
				if !bytes.Equal(out.Bytes(), want) {
					t.Errorf("printed %q, %s holds %q", out.Bytes(), p.expected, want)
				}
			})
		}
	}
}

// TestFibIsTheCxirExample checks that the CX source of the fib apps is the
// program of the cx-forth example whose IR the forth app translates.
func TestFibIsTheCxirExample(t *testing.T) {
	src, err := os.ReadFile("../cx-source-fib15/app/main.cx")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../cx-source-fib15/expected/stdout.txt")
	if err != nil {
		t.Fatal(err)
	}
	for _, ex := range cxir.Examples {
		if ex.Name == "fib" {
			if !bytes.Contains(src, []byte("r = fib(n-1) + fib(n-2)")) || !bytes.Contains(src, []byte("i32.print(fib(i))")) {
				t.Errorf("main.cx is not the example program:\n%s", ex.Source)
			}
			if ex.Output != string(want) {
				t.Errorf("the example prints %q, the expected file holds %q", ex.Output, want)
			}
			return
		}
	}
	t.Fatal("no fib example in cxir")
}
