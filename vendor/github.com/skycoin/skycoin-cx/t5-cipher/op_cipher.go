//go:build !nocipher

package cipher

import (
	"fmt"
	"hash"

	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	"github.com/skycoin/skycoin-cx/t0-cxhash"
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin/src/cipher"
)

// CipherHashStateCountMax is the size of the incremental hash table. A
// program that holds more states at once fails with an assert.
const CipherHashStateCountMax = 64

// CipherState holds the incremental hashes in a fixed table an i32 handle
// indexes. Each hash is made once and reset when its slot is taken again, so
// a run allocates nothing for it.
type CipherState struct {
	CipherHashStateTable [CipherHashStateCountMax]hash.Hash
	CipherHashStateTaken [CipherHashStateCountMax]bool
	// CipherDigestScratch receives HashStateSum, so the sum allocates nothing.
	CipherDigestScratch [cxhash.CxHashSize]byte
}

// VIOLATION: state outside State. Owner: t5-cipher; moves to a per program state with the cxfx tables, see D45.
var cipherState CipherState

// cipherStatePrepare makes every hash of the table once, before a run.
func cipherStatePrepare() {
	for handle := range cipherState.CipherHashStateTable {
		if cipherState.CipherHashStateTable[handle] == nil {
			cipherState.CipherHashStateTable[handle] = cxhash.CxHashNew()
		}
	}
}

// cipherHashStateAssertFailure ends the program for a handle out of range or
// naming a released or never created state.
// VIOLATION STRHOT-326: E, the assert text of a bad handle, built only when it fails, #510.
func cipherHashStateAssertFailure(handle int32) {
	assert.AssertPreconditionTrue(false, fmt.Sprintf("cipher hash state %d is out of range, released or never created", handle))
}

// cipherHashStateFullAssertFailure ends the program when every state is taken.
// VIOLATION STRHOT-327: E, the assert text of a full table, built only when it fails, #510.
func cipherHashStateFullAssertFailure() {
	assert.AssertPreconditionTrue(false, fmt.Sprintf("cipher hash state table is full, it holds %d", CipherHashStateCountMax))
}

// cipherHashStateGet returns the taken state handle names.
func cipherHashStateGet(handle int32) hash.Hash {
	if uint32(handle) >= CipherHashStateCountMax || !cipherState.CipherHashStateTaken[handle] {
		cipherHashStateAssertFailure(handle)
	}
	return cipherState.CipherHashStateTable[handle]
}

// cipherHashStateNew takes a free state, reset, and returns its handle.
func cipherHashStateNew() int32 {
	for handle := range cipherState.CipherHashStateTaken {
		if !cipherState.CipherHashStateTaken[handle] {
			cipherState.CipherHashStateTaken[handle] = true
			cipherState.CipherHashStateTable[handle].Reset()
			return int32(handle)
		}
	}
	cipherHashStateFullAssertFailure()
	return -1
}

// cipherHashStateRelease frees the state handle names.
func cipherHashStateRelease(handle int32) {
	cipherHashStateGet(handle)
	cipherState.CipherHashStateTaken[handle] = false
}

// cipherBytesView returns the elements of the []ui8 value, a view of program
// memory. An allocation may move the slice, so a view is used before one.
func cipherBytesView(prgrm *ast.CXProgram, value *ast.CXValue) []byte {
	return ast.GetSliceData(prgrm, value.Get_ptr(prgrm), 1)
}

// cipherBytesOutput writes a new []ui8 holding data to output. data is never
// a view of program memory, which the allocation may move.
// GCREACH: allocates the output byte slice, which adds to the GC pressure,
// see docs/GarbageCollector.md.
func cipherBytesOutput(prgrm *ast.CXProgram, output *ast.CXValue, data []byte) {
	slice := ast.SliceResizeEx(prgrm, 0, types.Cast_int_to_ptr(len(data)), 1)
	copy(ast.GetSliceData(prgrm, slice, 1), data)
	output.Set_ptr(prgrm, slice)
}

// cipherArrayFromView copies view into array and reports whether its length
// is the length of array.
func cipherArrayFromView(array []byte, view []byte) bool {
	if len(view) != len(array) {
		return false
	}
	copy(array, view)
	return true
}

// CipherSignatureVerify reports whether signature is a valid signature of
// hash by pubKey, as cipher.VerifyPubKeySignedHash of Skycoin decides. A
// hash, signature or key of the wrong length is not valid.
func CipherSignatureVerify(hashView []byte, signatureView []byte, pubKeyView []byte) bool {
	var digest cipher.SHA256
	var signature cipher.Sig
	var pubKey cipher.PubKey
	if !cipherArrayFromView(digest[:], hashView) || !cipherArrayFromView(signature[:], signatureView) || !cipherArrayFromView(pubKey[:], pubKeyView) {
		return false
	}
	return cipher.VerifyPubKeySignedHash(pubKey, signature, digest) == nil
}

// OpCipherHashOfBytes is cipher.HashOfBytes, the digest of a []ui8.
func OpCipherHashOfBytes(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	digest := cxhash.CxHashOfBytes(cipherBytesView(prgrm, &inputs[0]))
	cipherBytesOutput(prgrm, &outputs[0], digest[:])
}

// OpCipherHashStateNew is cipher.HashStateNew, a new incremental hash.
func OpCipherHashStateNew(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, cipherHashStateNew())
}

// OpCipherHashStateWrite is cipher.HashStateWrite, which adds a []ui8 to a
// state.
func OpCipherHashStateWrite(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cipherHashStateGet(inputs[0].Get_i32(prgrm)).Write(cipherBytesView(prgrm, &inputs[1]))
}

// OpCipherHashStateSum is cipher.HashStateSum, the digest of what was written
// to a state. The state stays as it is and takes more writes.
func OpCipherHashStateSum(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	state := cipherHashStateGet(inputs[0].Get_i32(prgrm))
	digest := state.Sum(cipherState.CipherDigestScratch[:0])
	cipherBytesOutput(prgrm, &outputs[0], digest)
}

// OpCipherHashStateRelease is cipher.HashStateRelease, which frees a state.
func OpCipherHashStateRelease(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	cipherHashStateRelease(inputs[0].Get_i32(prgrm))
}

// OpCipherSignatureVerify is cipher.SignatureVerify(hash, signature, pubKey).
// VIOLATION VMALLOC-024: secp256k1-go2 of Skycoin computes with math/big, #510.
func OpCipherSignatureVerify(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	valid := CipherSignatureVerify(cipherBytesView(prgrm, &inputs[0]), cipherBytesView(prgrm, &inputs[1]), cipherBytesView(prgrm, &inputs[2]))
	outputs[0].Set_bool(prgrm, valid)
}

// OpCipherSignatureRecoverPubKey is cipher.SignatureRecoverPubKey(hash,
// signature), the public key that made signature, and false when there is
// none.
// VIOLATION VMALLOC-024: secp256k1-go2 of Skycoin computes with math/big, #510.
func OpCipherSignatureRecoverPubKey(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var digest cipher.SHA256
	var signature cipher.Sig
	var pubKey cipher.PubKey
	recovered := cipherArrayFromView(digest[:], cipherBytesView(prgrm, &inputs[0])) && cipherArrayFromView(signature[:], cipherBytesView(prgrm, &inputs[1]))
	if recovered {
		var err error
		pubKey, err = cipher.PubKeyFromSig(signature, digest)
		recovered = err == nil
	}
	cipherSliceOutput(prgrm, &outputs[0], pubKey[:], recovered)
	outputs[1].Set_bool(prgrm, recovered)
}

// OpCipherSignatureSign is cipher.SignatureSign(hash, secKey), the signature
// of hash, and false for a secret key that is not valid or a hash of zeros.
// VIOLATION VMALLOC-024: secp256k1-go2 of Skycoin computes with math/big, #510.
func OpCipherSignatureSign(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var digest cipher.SHA256
	var secKey cipher.SecKey
	var signature cipher.Sig
	signed := cipherArrayFromView(digest[:], cipherBytesView(prgrm, &inputs[0])) && cipherArrayFromView(secKey[:], cipherBytesView(prgrm, &inputs[1]))
	if signed {
		var err error
		signature, err = cipher.SignHash(digest, secKey)
		signed = err == nil
	}
	cipherSliceOutput(prgrm, &outputs[0], signature[:], signed)
	outputs[1].Set_bool(prgrm, signed)
}

// OpCipherKeyPairGenerate is cipher.KeyPairGenerate, a new random public and
// secret key.
// VIOLATION VMALLOC-024: secp256k1-go2 of Skycoin computes with math/big, #510.
func OpCipherKeyPairGenerate(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	pubKey, secKey := cipher.GenerateKeyPair()
	cipherBytesOutput(prgrm, &outputs[0], pubKey[:])
	cipherBytesOutput(prgrm, &outputs[1], secKey[:])
}

// cipherSliceOutput writes data as a new []ui8 to output when written, and
// the nil slice otherwise.
// GCREACH: allocates the output byte slice through cipherBytesOutput, see
// docs/GarbageCollector.md.
func cipherSliceOutput(prgrm *ast.CXProgram, output *ast.CXValue, data []byte, written bool) {
	if !written {
		output.Set_ptr(prgrm, 0)
		return
	}
	cipherBytesOutput(prgrm, output, data)
}

// LibraryCipherHashStateNewVm is cipher.HashStateNew on vm, the i32 handle at
// block.
func LibraryCipherHashStateNewVm(memory []byte, block uint32) {
	types.Write_i32(memory, types.Pointer(block), cipherHashStateNew())
}

// LibraryCipherHashStateReleaseVm is cipher.HashStateRelease on vm, the i32
// handle at block.
func LibraryCipherHashStateReleaseVm(memory []byte, block uint32) {
	cipherHashStateRelease(types.Read_i32(memory, types.Pointer(block)))
}
