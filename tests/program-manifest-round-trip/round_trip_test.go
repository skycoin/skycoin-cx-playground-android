// Package programmanifestroundtrip builds the manifest of program/ and
// checks it against manifest.txt, the example envelope of
// docs/ProgramDistribution.md: it decodes, encodes back to the same bytes,
// has the address and text of a fresh build, and the program runs.
package programmanifestroundtrip

import (
	"bytes"
	"flag"
	"os"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/apprun"
	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

var update = flag.Bool("update", false, "rewrite manifest.txt from program/")

// The example signer and date. The key is derived from a public seed, so
// it is for tests only.
var (
	exampleSeed = []byte("cx playground example signer")
	exampleDate = time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
)

func build(t *testing.T) (*programblocks.Manifest, []byte, map[string][]byte) {
	files, err := programblocks.ReadProgramDirectory("program")
	if err != nil {
		t.Fatal(err)
	}
	m, pack, err := programblocks.Build(files, programblocks.DefaultBlockSize, exampleDate)
	if err != nil {
		t.Fatal(err)
	}
	_, secret := cipher.MustGenerateDeterministicKeyPair(exampleSeed)
	if err := m.Sign(secret); err != nil {
		t.Fatal(err)
	}
	return m, pack, files
}

func TestRoundTrip(t *testing.T) {
	m, pack, files := build(t)
	if *update {
		if err := os.WriteFile("manifest.txt", m.Encode(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	envelope, err := os.ReadFile("manifest.txt")
	if err != nil {
		t.Fatal(err)
	}
	decoded, addr, err := programblocks.Decode(envelope, programblocks.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Encode(); !bytes.Equal(got, envelope) {
		t.Fatalf("envelope does not encode back the same:\n%s", got)
	}
	// The signature is not part of the address, so a fresh build with the
	// same date has the address and text of the committed example.
	if m.Address() != addr || !bytes.Equal(m.EncodeText(), decoded.EncodeText()) {
		t.Fatalf("manifest.txt is stale, run go test -update: address %s, built %s", addr, m.Address())
	}
	if pub, _ := cipher.MustGenerateDeterministicKeyPair(exampleSeed); decoded.SignerPublicKey != pub {
		t.Fatalf("signature recovers to %s, want %s", decoded.SignerPublicKey.Hex(), pub.Hex())
	}
	if len(decoded.Blocks) != 3 || len(decoded.Files) != 2 || decoded.PackLength != uint64(len(pack)) {
		t.Fatalf("%d files, %d blocks, %d bytes: the example should show one file of two blocks", len(decoded.Files), len(decoded.Blocks), decoded.PackLength)
	}
	for _, f := range decoded.Files {
		if !bytes.Equal(pack[f.PackOffset:f.PackOffset+f.FileLength], files[f.Path]) {
			t.Fatalf("%s is not at its offset in the linear packing", f.Path)
		}
	}

	app, err := programblocks.AppManifest(files)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := apprun.Run(app, files, cxrun.Options{}, nil, &out); err != nil {
		t.Fatalf("%v\n%s", err, out.Bytes())
	}
	want, err := os.ReadFile("expected/stdout.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("printed %q, want %q", out.Bytes(), want)
	}
}

// TestCompressedRoundTrip checks the #lz envelope: it decodes to the same
// manifest, and its address is the hash of the LZ4 bytes, not of the text.
func TestCompressedRoundTrip(t *testing.T) {
	m, _, _ := build(t)
	plain := m.Address()
	m.Compressed = true
	_, secret := cipher.MustGenerateDeterministicKeyPair(exampleSeed)
	if err := m.Sign(secret); err != nil {
		t.Fatal(err)
	}
	b := m.Encode()
	if !bytes.HasPrefix(b, []byte(programblocks.Magic+" 3 "+programblocks.CompressedMark+"\n")) {
		t.Fatalf("header line of %q", b[:16])
	}
	decoded, addr, err := programblocks.Decode(b, programblocks.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	if addr == plain || addr != programblocks.Sum(m.Data()) || !decoded.Compressed {
		t.Fatalf("address %s, text address %s", addr, plain)
	}
	if !bytes.Equal(decoded.EncodeText(), m.EncodeText()) || !bytes.Equal(decoded.Encode(), b) {
		t.Fatal("compressed envelope does not round trip")
	}
}
