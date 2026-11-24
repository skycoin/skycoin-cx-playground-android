// Package programtestsuitereportformat checks the result line of the test
// programs (#19): lines read back unchanged and malformed ones are refused.
package programtestsuitereportformat

import (
	"errors"
	"testing"

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
