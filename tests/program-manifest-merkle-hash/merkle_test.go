// Package programmanifestmerklehash checks the Merkle hash of
// docs/ProgramDistribution.md against roots computed by hand from BLAKE3-256:
// inner nodes hash 0x01, left and right, and an odd last node moves up.
package programmanifestmerklehash

import (
	"encoding/hex"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"lukechampine.com/blake3"
)

func inner(l, r programblocks.Hash) programblocks.Hash {
	return blake3.Sum256(append(append([]byte{1}, l[:]...), r[:]...))
}

func TestMerkleHash(t *testing.T) {
	var h [5]programblocks.Hash
	for i := range h {
		h[i] = blake3.Sum256([]byte{byte(i)})
	}
	cases := []struct {
		leaves []programblocks.Hash
		want   programblocks.Hash
	}{
		{nil, blake3.Sum256(nil)},
		{h[:1], h[0]},
		{h[:2], inner(h[0], h[1])},
		{h[:3], inner(inner(h[0], h[1]), h[2])},
		{h[:4], inner(inner(h[0], h[1]), inner(h[2], h[3]))},
		{h[:5], inner(inner(inner(h[0], h[1]), inner(h[2], h[3])), h[4])},
	}
	for _, c := range cases {
		if got := programblocks.MerkleHash(c.leaves); got != c.want {
			t.Errorf("%d leaves: %s, want %s", len(c.leaves), got, c.want)
		}
	}
	// Pairing the odd node with itself would give another root.
	if programblocks.MerkleHash(h[:3]) == inner(inner(h[0], h[1]), inner(h[2], h[2])) {
		t.Error("odd node is paired with itself")
	}
}

// Sum is BLAKE3-256. The vectors are the hash mode vectors of BLAKE3, the
// first 32 bytes of each output, of the repeating sequence 0, 1, ..., 250,
// https://github.com/BLAKE3-team/BLAKE3/blob/master/test_vectors/test_vectors.json
func TestSumVectors(t *testing.T) {
	vectors := []struct {
		length int
		digest string
	}{
		{0, "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262"},
		{1, "2d3adedff11b61f14c886e35afa036736dcd87a74d27b5c1510225d0f592e213"},
		{1023, "10108970eeda3eb932baac1428c7a2163b0e924c9a9e25b35bba72b28f70bd11"},
		{1024, "42214739f095a406f3fc83deb889744ac00df831c10daa55189b5d121c855af7"},
		{1025, "d00278ae47eb27b34faecf67b4fe263f82d5412916c1ffd97c8cb7fb814b8444"},
		{102400, "bc3e3d41a1146b069abffad3c0d44860cf664390afce4d9661f7902e7943e085"},
	}
	for _, v := range vectors {
		input := make([]byte, v.length)
		for i := range input {
			input[i] = byte(i % 251)
		}
		if got := programblocks.Sum(input); hex.EncodeToString(got[:]) != v.digest {
			t.Errorf("Sum of %d bytes = %s, want %s", v.length, got, v.digest)
		}
	}
}
