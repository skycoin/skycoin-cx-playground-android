// Package programtestsuitereportformat checks the result line of the test
// programs (#19): lines read back unchanged, malformed ones are refused, and
// both bundled test programs, run as the app runs them, print only well
// formed lines that pass. On the phone the serial and the direct block
// tests (D29, ESP32 only) report skipped.
package programtestsuitereportformat

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/store"
	"github.com/skycoin/skycoin-cx-playground-android/testreport"
)

func TestRoundTrip(t *testing.T) {
	for _, line := range []string{
		"HashSha256Known true 703 43 1422.185 block=64B vectors=4",
		"StorageDirectBlockRead1024 false 38936 38936 25.682 block=1024B blocks=64 failures=0 mismatches=7",
		"MemoryPatternWalkingOnes true 1485570 5803 0.673",
		"SerialUartRead skipped - - - - - no UART on the phone",
		"SerialSpiWrite true 10 5 100.5 2000 1800.25",
	} {
		r, err := testreport.ParseLine(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if got := r.String(); got != line {
			t.Errorf("read back %q, want %q", got, line)
		}
	}
	r, _ := testreport.ParseLine("SerialSpiWrite true 10 5 100.5 2000 1800.25 loopback")
	if v, ok := r.SerialWriteKBPerSecond.Value(); !ok || v != 1800.25 || r.Note != "loopback" {
		t.Errorf("serial write %v %v, note %q", v, ok, r.Note)
	}
	if _, ok := r.BlocksPerSecond.Value(); !ok {
		t.Error("blocks per second not measured")
	}
	r, _ = testreport.ParseLine("SerialUartRead skipped - - - - -")
	if _, ok := r.LatencyMicroseconds.Value(); ok || !r.Passed() {
		t.Errorf("skipped line: %+v", r)
	}
}

func TestMalformed(t *testing.T) {
	for _, line := range []string{
		"hashSha256Known true 1 2 3",
		"Hash-Sha256 true 1 2 3",
		"HashSha256Known true 1 2",
		"HashSha256Known true 1 x 3",
		"HashSha256Known true 1 -2 3",
		"HashSha256Known true 1 1e3 3",
		"SerialUartRead skipped - - - -",
	} {
		if _, err := testreport.ParseLine(line); err == nil || errors.Is(err, testreport.ErrNotResult) {
			t.Errorf("%q: %v, want a malformed line error", line, err)
		}
	}
	for _, line := range []string{"", "test program done", "memtest pass"} {
		if _, err := testreport.ParseLine(line); !errors.Is(err, testreport.ErrNotResult) {
			t.Errorf("%q: %v, want ErrNotResult", line, err)
		}
	}
}

// runBundled runs a bundled app from the app's bundle as the app runs it:
// every library and a fresh sandbox root.
func runBundled(t *testing.T, name string) string {
	src := store.FSSource{FS: os.DirFS("../../cmd/cxplayground/bundle")}
	apps, errs, err := store.List(src)
	if err != nil || len(errs) > 0 {
		t.Fatal(err, errs)
	}
	i := slices.IndexFunc(apps, func(a store.App) bool { return a.Manifest.Name == name })
	if i < 0 {
		t.Fatalf("no %q app in the bundle", name)
	}
	files, err := store.Load(src, apps[i])
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	opts := cxrun.Options{Dir: t.TempDir(), Libraries: cxrun.LibrariesAll()}
	if err := cxrun.RunWith(files, apps[i].Manifest.Entry, opts, nil, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	return out.String()
}

func TestBundledProgramsReport(t *testing.T) {
	for _, c := range []struct {
		app   string
		names []string
	}{
		{"Test program: hash and memory", []string{
			"HashMerkleKnown", "HashSha256Known", "MemoryCorruptionDetectedBitFlip",
			"MemoryPatternAddress", "MemoryPatternMovingInversions", "MemoryPatternRandomSeeded",
			"MemoryPatternWalkingOnes", "MemoryPatternWalkingZeros",
		}},
		{"Test program: storage and serial", []string{
			"SerialSpiRead", "SerialSpiWrite", "SerialUartRead", "SerialUartWrite",
			"StorageCorruptionDetectedByteFlip",
			"StorageDirectBlockRead1024", "StorageDirectBlockRead2048", "StorageDirectBlockRead4096", "StorageDirectBlockRead512",
			"StorageDirectBlockWrite1024", "StorageDirectBlockWrite2048", "StorageDirectBlockWrite4096", "StorageDirectBlockWrite512",
			"StorageFileSystemRead1024", "StorageFileSystemRead2048", "StorageFileSystemRead4096", "StorageFileSystemRead512",
			"StorageFileSystemWrite1024", "StorageFileSystemWrite2048", "StorageFileSystemWrite4096", "StorageFileSystemWrite512",
		}},
	} {
		out := runBundled(t, c.app)
		t.Logf("%s\n%s", c.app, out)
		results, err := testreport.ParseOutput(strings.Split(strings.TrimSpace(out), "\n"))
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, r := range results {
			names = append(names, r.Name)
			if !r.Passed() {
				t.Errorf("%s", r)
			}
			if r.Verdict == testreport.VerdictSkipped && !r.Serial() && !strings.HasPrefix(r.Name, "StorageDirectBlock") {
				t.Errorf("only the serial and direct block tests skip on the phone: %s", r)
			}
		}
		slices.Sort(names)
		if !slices.Equal(names, c.names) {
			t.Errorf("%s: tests %q, want %q", c.app, names, c.names)
		}
	}
}
