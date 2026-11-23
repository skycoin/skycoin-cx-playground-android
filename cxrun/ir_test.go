package cxrun

import (
	"bytes"
	"path/filepath"
	"testing"
)

// TestIRMatchesSource runs programs from source on the old executor and from
// their serialized form on every executor, and wants the same output.
func TestIRMatchesSource(t *testing.T) {
	for _, file := range []string{"fib.cx", "primes.cx", "struct.cx", "count.cx"} {
		t.Run(file, func(t *testing.T) {
			files := map[string][]byte{"main.cx": readApp(t, filepath.Join("..", "apps", "benchmarks"))[file]}
			var want bytes.Buffer
			if _, err := RunOn(files, "main.cx", ExecutorOld, nil, &want); err != nil {
				t.Fatalf("source: %v\n%s", err, want.Bytes())
			}
			program, err := Serialize(files, "main.cx")
			if err != nil {
				t.Fatal(err)
			}
			again, err := Serialize(files, "main.cx")
			if err != nil || !bytes.Equal(program, again) {
				t.Fatalf("serializing twice gave different bytes (%v)", err)
			}
			for _, ex := range []Executor{ExecutorAuto, ExecutorOld, ExecutorVm} {
				var got bytes.Buffer
				if _, err := RunIRWith(program, ex, Options{}, nil, &got); err != nil {
					t.Fatalf("ir on %s: %v\n%s", ex, err, got.Bytes())
				}
				if !bytes.Equal(got.Bytes(), want.Bytes()) {
					t.Errorf("ir on %s printed %q, source printed %q", ex, got.Bytes(), want.Bytes())
				}
			}
		})
	}
}

// TestIRAppPackage runs a program that uses the app package from its
// serialized form. The app opcodes are outside the vm, so auto takes the old
// executor.
func TestIRAppPackage(t *testing.T) {
	files := readApp(t, filepath.Join("..", "tests", "ui-key-input", "app"))
	program, err := Serialize(files, "main.cx")
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHost{events: []Event{{Kind: EvKey, Code: 'a'}, {Kind: EvKey, Code: KeyEnter}}}
	var out bytes.Buffer
	if _, err := RunIRWith(program, ExecutorAuto, Options{}, h, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.Bytes())
	}
	if want := "key 1 code=97 97\nkey 2 code=10 enter\n"; out.String() != want {
		t.Errorf("output %q, want %q", out.String(), want)
	}
}
