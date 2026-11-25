// Package cxhash is the one hash function of CX, used by the cipher system
// library and the execution records. It is BLAKE3 with a 256 bit output,
// decision D23 of skycoin-cx#510. Changing it changes CxHashName, CxHashNew
// and CxHashOfBytes in this file only.
package cxhash

import (
	"hash"

	"lukechampine.com/blake3"
)

// CxHashName names the hash, as the execution records file names it.
const CxHashName = "BLAKE3-256"

// CxHashSize is the size of a digest in bytes.
const CxHashSize = 32

// CxHashNew returns a new incremental hash.
func CxHashNew() hash.Hash {
	return blake3.New(CxHashSize, nil)
}

// CxHashOfBytes returns the digest of data. It allocates nothing.
func CxHashOfBytes(data []byte) [CxHashSize]byte {
	return blake3.Sum256(data)
}
