// Package programmanifestwrongsignature checks that a manifest envelope is
// rejected when its signature or hash was changed or any data byte changed
// after signing, and that a signature by another key still matches (D27).
package programmanifestwrongsignature

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func build(t *testing.T) *programblocks.Manifest {
	files, err := programblocks.ReadProgramDirectory("../program-manifest-round-trip/program")
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := programblocks.Build(files, programblocks.DefaultBlockSize, time.Unix(1795478400, 0))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func signed(t *testing.T, seed string) *programblocks.Manifest {
	m := build(t)
	_, secret := cipher.MustGenerateDeterministicKeyPair([]byte(seed))
	if err := m.Sign(secret); err != nil {
		t.Fatal(err)
	}
	return m
}

// changed returns b with the hex digit at i of line changed to another
// hex digit.
func changed(b []byte, line, i int) []byte {
	out := append([]byte(nil), b...)
	start := 0
	for ; line > 0; line-- {
		start += bytes.IndexByte(out[start:], '\n') + 1
	}
	if out[start+i] == '0' {
		out[start+i] = '1'
	} else {
		out[start+i] = '0'
	}
	return out
}

func TestWrongSignatureDetected(t *testing.T) {
	good := signed(t, "signer one").Encode()
	if _, _, err := programblocks.Decode(good, programblocks.Hash{}); err != nil {
		t.Fatal(err)
	}
	// A recoverable signature changed in one digit either recovers no key or
	// recovers another key, which a caller that pins the signer refuses.
	m, _, err := programblocks.Decode(changed(good, 1, 5), programblocks.Hash{})
	if pub, _ := cipher.MustGenerateDeterministicKeyPair([]byte("signer one")); err == nil && m.SignerPublicKey == pub {
		t.Errorf("changed signature recovers the signer")
	} else if err != nil && !errors.Is(err, programblocks.ErrSignature) {
		t.Errorf("changed signature: %v", err)
	}
	// A changed hash line no longer names the data.
	if _, _, err := programblocks.Decode(changed(good, 2, 5), programblocks.Hash{}); !errors.Is(err, programblocks.ErrDataHash) {
		t.Errorf("changed hash: %v", err)
	}
	// The date is in the data, so changing it fails the hash of the data.
	m = build(t)
	m.Date++
	date := append([]byte(nil), good...)
	text := build(t).EncodeText()
	copy(date[len(date)-len(text):], m.EncodeText())
	if _, _, err := programblocks.Decode(date, programblocks.Hash{}); !errors.Is(err, programblocks.ErrDataHash) {
		t.Errorf("changed date: %v", err)
	}
	last := append([]byte(nil), good...)
	last[len(last)-2] ^= 1
	if _, _, err := programblocks.Decode(last, programblocks.Hash{}); !errors.Is(err, programblocks.ErrDataHash) {
		t.Errorf("changed data byte: %v", err)
	}
}

// TestOtherSignerMatches checks D27: a signature by any key matches, and
// the key it recovers to is reported for a caller that checks who signed.
func TestOtherSignerMatches(t *testing.T) {
	one, two := signed(t, "signer one"), signed(t, "signer two")
	if one.Address() != two.Address() {
		t.Fatal("the signer changed the address")
	}
	m, _, err := programblocks.Decode(two.Encode(), one.Address())
	if err != nil {
		t.Fatal(err)
	}
	if pub, _ := cipher.MustGenerateDeterministicKeyPair([]byte("signer two")); m.SignerPublicKey != pub {
		t.Fatalf("recovered %s, want %s", m.SignerPublicKey.Hex(), pub.Hex())
	}
}
