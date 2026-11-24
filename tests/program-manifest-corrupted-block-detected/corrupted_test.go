// Package programmanifestcorruptedblock checks that one changed byte in any
// block of the linear packing is caught when the block arrives, and that a
// good copy of the block is then accepted.
package programmanifestcorruptedblock

import (
	"errors"
	"testing"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func TestCorruptedBlockDetected(t *testing.T) {
	files, err := programblocks.ReadProgramDirectory("../program-manifest-round-trip/program")
	if err != nil {
		t.Fatal(err)
	}
	m, pack, err := programblocks.Build(files, programblocks.DefaultBlockSize, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range m.Packing {
		bad := append([]byte(nil), pack...)
		bad[p.PackOffset+uint64(p.BlockLength)/2] ^= 0x20
		a := programblocks.NewAssembly(m)
		if _, err := a.Accept(0, bad); !errors.Is(err, programblocks.ErrBlockHash) {
			t.Fatalf("block %d: corruption not detected: %v", p.BlockID, err)
		}
		if _, err := a.Accept(0, pack); err != nil {
			t.Fatal(err)
		}
		got, err := a.Files()
		if err != nil {
			t.Fatal(err)
		}
		for path, b := range files {
			if string(got[path]) != string(b) {
				t.Fatalf("%s differs after the good copy", path)
			}
		}
	}
}

// TestPartialRangeIgnored checks that a range ending inside a block stores
// only the whole blocks before it.
func TestPartialRangeIgnored(t *testing.T) {
	files, _ := programblocks.ReadProgramDirectory("../program-manifest-round-trip/program")
	m, pack, _ := programblocks.Build(files, programblocks.DefaultBlockSize, time.Unix(0, 0))
	a := programblocks.NewAssembly(m)
	end := m.Packing[1].PackOffset + 10
	n, err := a.Accept(0, pack[:end])
	if err != nil || n != 1 {
		t.Fatalf("stored %d blocks, err %v; want 1", n, err)
	}
	if r := a.MissingRanges(1152); len(r) != 1 || r[0][0] != m.Packing[1].PackOffset {
		t.Fatalf("missing ranges %v", r)
	}
}
