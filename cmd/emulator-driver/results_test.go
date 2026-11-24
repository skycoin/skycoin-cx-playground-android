package main

import (
	"strings"
	"testing"
)

func TestCheckResults(t *testing.T) {
	traces := []string{
		"start app=\"Test program: storage and serial\" hash=x",
		"out test program storage and serial",
		"out SerialUartRead skipped - - - - - no UART on the phone",
		"out StorageDirectBlockWrite512 true 280 140 3559.759 block=512B blocks=64 failures=0 mismatches=0",
		"out StorageDirectBlockRead512 false 47619 23809 20.999 block=512B blocks=64 failures=0 mismatches=3",
		"frame n=1 event=0",
	}
	results, err := programResults(traces)
	if err != nil || len(results) != 3 {
		t.Fatalf("%d results, %v", len(results), err)
	}
	expect, err := parseExpectations(strings.NewReader("result SerialUartRead skipped\nresult StorageDirectBlockWrite512 true\nout test program\n"))
	if err != nil {
		t.Fatal(err)
	}
	if f := checkResults(expect, results); len(f) != 0 {
		t.Fatalf("failures %q", f)
	}
	expect, err = parseExpectations(strings.NewReader("result StorageDirectBlockRead512 true\nresult StorageFileSystemRead512 true\nresults-pass\n"))
	if err != nil {
		t.Fatal(err)
	}
	f := checkResults(expect, results)
	if len(f) != 3 || !strings.Contains(f[0], "got StorageDirectBlockRead512 false") || !strings.Contains(f[1], "no result line") || !strings.Contains(f[2], "results-pass") {
		t.Fatalf("failures %q", f)
	}
	for _, bad := range []string{"result X", "result X maybe", "results-pass now"} {
		if _, err := parseExpectations(strings.NewReader(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	if _, err := programResults([]string{"out StorageDirectBlockRead512 true 1 2"}); err == nil {
		t.Error("a result line without its latency parsed")
	}
}
