// Package programmanifestmerklehash checks the Merkle hash of
// docs/ProgramDistribution.md against roots computed by hand from SHA-256:
// inner nodes hash 0x01, left and right, and an odd last node moves up.
package programmanifestmerklehash

import (
	"crypto/sha256"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func inner(l, r programblocks.Hash) programblocks.Hash {
	return sha256.Sum256(append(append([]byte{1}, l[:]...), r[:]...))
}

func TestMerkleHash(t *testing.T) {
	var h [5]programblocks.Hash
	for i := range h {
		h[i] = sha256.Sum256([]byte{byte(i)})
	}
	cases := []struct {
		leaves []programblocks.Hash
		want   programblocks.Hash
	}{
		{nil, sha256.Sum256(nil)},
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
