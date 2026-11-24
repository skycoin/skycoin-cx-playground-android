// Package cxhash is the one hash function of CX, used by the cipher system
// library and the execution records. Which hash it is, is decision D23 of
// skycoin-cx#510. The lead asked for a BLAKE hash and the variant is not
// confirmed, so it is SHA-256 until then. Changing it changes CxHashName,
// CxHashNew and CxHashOfBytes in this file only.
package cxhash

import (
	"crypto/sha256"
	"hash"
)

// CxHashName names the hash, as the execution records file names it.
const CxHashName = "SHA-256"

// CxHashSize is the size of a digest in bytes. Every candidate of D23 has a
// 256 bit digest.
const CxHashSize = 32

// CxHashNew returns a new incremental hash.
func CxHashNew() hash.Hash {
	return sha256.New()
}

// CxHashOfBytes returns the digest of data. It allocates nothing.
func CxHashOfBytes(data []byte) [CxHashSize]byte {
	return sha256.Sum256(data)
}
