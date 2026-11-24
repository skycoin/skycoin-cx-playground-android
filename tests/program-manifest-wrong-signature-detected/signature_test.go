// Package programmanifestwrongsignature checks that a manifest is rejected
// when its signature was changed, when it names a key that did not sign it,
// and when any signed byte changed after signing.
package programmanifestwrongsignature

import (
	"errors"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func signed(t *testing.T) []byte {
	files, err := programblocks.ReadProgramDirectory("../program-manifest-round-trip/program")
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := programblocks.Build(files, programblocks.DefaultBlockSize, time.Unix(1795478400, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, secret := cipher.MustGenerateDeterministicKeyPair([]byte("signer one"))
	if err := m.Sign(secret); err != nil {
		t.Fatal(err)
	}
	return m.Encode()
}

func TestWrongSignatureDetected(t *testing.T) {
	good := signed(t)
	if _, _, err := programblocks.Decode(good, programblocks.Hash{}); err != nil {
		t.Fatal(err)
	}
	sig := programblocks.HeaderLengthVersion1 - programblocks.SignatureLength
	key := sig - programblocks.PublicKeyLength

	changedSig := append([]byte(nil), good...)
	changedSig[sig+5] ^= 1
	if _, _, err := programblocks.Decode(changedSig, programblocks.Hash{}); !errors.Is(err, programblocks.ErrSignature) {
		t.Errorf("changed signature: %v", err)
	}

	otherKey := append([]byte(nil), good...)
	pub, _ := cipher.MustGenerateDeterministicKeyPair([]byte("signer two"))
	copy(otherKey[key:], pub[:])
	if _, _, err := programblocks.Decode(otherKey, programblocks.Hash{}); !errors.Is(err, programblocks.ErrSignature) {
		t.Errorf("other signer key: %v", err)
	}

	// The date is signed: changing it changes the address the signature is over.
	changedDate := append([]byte(nil), good...)
	changedDate[15] ^= 1
	if _, _, err := programblocks.Decode(changedDate, programblocks.Hash{}); !errors.Is(err, programblocks.ErrSignature) {
		t.Errorf("changed date: %v", err)
	}

	// An array byte is covered through its hash in the signed directory.
	changedArray := append([]byte(nil), good...)
	changedArray[len(good)-1] ^= 1
	if _, _, err := programblocks.Decode(changedArray, programblocks.Hash{}); !errors.Is(err, programblocks.ErrArrayHash) {
		t.Errorf("changed array byte: %v", err)
	}
}
