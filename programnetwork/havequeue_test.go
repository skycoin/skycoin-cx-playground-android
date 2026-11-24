package programnetwork

import (
	"testing"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// TestHaveQueueBatches checks the update model of skycoin-cx#508 on the have
// queue: nothing is visible before an update, a batch is sorted by sender and
// keeps the arrival order of one sender, and haves that arrive after an
// update wait for the next one.
func TestHaveQueueBatches(t *testing.T) {
	q := newHaveQueue()
	have := func(from string, b byte) QueuedHave {
		return QueuedHave{Have: Have{Address: programblocks.Hash{b}}, From: from}
	}
	q.push(have("10.0.0.2:7010", 1))
	q.push(have("10.0.0.1:7010", 2))
	q.push(have("10.0.0.2:7010", 3))
	q.push(have("10.0.0.1:7010", 4))
	if q.ReadyCount() != 0 || q.Pending() != 4 {
		t.Fatalf("before an update: ready %d, pending %d, want 0 and 4", q.ReadyCount(), q.Pending())
	}
	if n := q.Update(0); n != 4 {
		t.Fatalf("update has %d haves, want 4", n)
	}
	q.push(have("10.0.0.0:7010", 5))
	var got []byte
	for h, ok := q.ReadyNext(); ok; h, ok = q.ReadyNext() {
		got = append(got, h.Address[0])
	}
	if want := []byte{2, 4, 1, 3}; string(got) != string(want) {
		t.Fatalf("batch order %v, want %v", got, want)
	}
	if q.Pending() != 1 || q.ReadyCount() != 0 {
		t.Fatalf("after reading: pending %d, ready %d, want 1 and 0", q.Pending(), q.ReadyCount())
	}
	if n := q.Update(time.Second); n != 1 {
		t.Fatalf("second update has %d haves, want 1", n)
	}
	if h, _ := q.ReadyNext(); h.Address[0] != 5 {
		t.Fatalf("second batch holds %v", h.Address)
	}
	// The first empty update may wake early on the signal of the have before.
	if n := q.Update(50 * time.Millisecond); n != 0 {
		t.Fatalf("empty update has %d haves", n)
	}
	start := time.Now()
	if q.Update(50*time.Millisecond) != 0 || time.Since(start) < 50*time.Millisecond {
		t.Fatalf("an empty update did not wait")
	}
	if q.Clock() < 50 {
		t.Fatalf("clock %d ms after waiting 50 ms", q.Clock())
	}
}
