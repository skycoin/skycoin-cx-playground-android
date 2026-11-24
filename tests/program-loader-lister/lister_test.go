// Test program-loader-lister writes an index of N programs and reads it
// back with the lister, checking every row and that the number of
// allocations does not grow with N.
package lister

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

func index(t *testing.T, n int) *programstore.Store {
	t.Helper()
	s, err := programstore.Open(filepath.Join(t.TempDir(), "programs"))
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]programstore.Row, n)
	states := []string{programstore.StateRemote, programstore.StatePartial, programstore.StateCached}
	for i := range rows {
		rows[i] = programstore.Row{Number: i + 1, Address: fmt.Sprintf("%064x", i*7919), State: states[i%3], Length: int64(512 * (i + 1)), Name: fmt.Sprintf("program %d with spaces", i+1)}
	}
	if err := s.WriteIndex(rows); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReadIndex()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("%d rows, want %d", len(got), n)
	}
	for i := range rows {
		if got[i] != rows[i] {
			t.Fatalf("row %d: %+v, want %+v", i, got[i], rows[i])
		}
	}
	return s
}

func TestListerReadsNPrograms(t *testing.T) {
	var allocs []float64
	for _, n := range []int{10, 1000} {
		s := index(t, n)
		allocs = append(allocs, testing.AllocsPerRun(20, func() {
			if _, err := s.ReadIndex(); err != nil {
				t.Fatal(err)
			}
		}))
		t.Logf("%d programs: %.0f allocations per read", n, allocs[len(allocs)-1])
	}
	if allocs[1] != allocs[0] {
		t.Fatalf("allocations grow with the rows: %v", allocs)
	}
	empty, err := programstore.Open(filepath.Join(t.TempDir(), "none"))
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := empty.ReadIndex(); err != nil || rows != nil {
		t.Fatalf("store without an index: %v %v", rows, err)
	}
	if _, err := programstore.ParseIndex("1 abc remote\n"); err == nil {
		t.Fatal("short line accepted")
	}
}
