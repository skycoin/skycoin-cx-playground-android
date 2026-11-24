// Package programmanifestroundtrip builds the manifest of program/ and
// checks it against manifest.ini, the example of docs/ProgramDistribution.md:
// the text form parses, prints back to the same bytes, gives the same binary
// form and address as a fresh build, and the program runs.
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

var update = flag.Bool("update", false, "rewrite manifest.ini from program/")

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
		if err := os.WriteFile("manifest.ini", m.EncodeText(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	text, err := os.ReadFile("manifest.ini")
	if err != nil {
		t.Fatal(err)
	}
	decoded, bin, addr, err := programblocks.DecodeText(text)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.EncodeText(); !bytes.Equal(got, text) {
		t.Fatalf("text form does not print back the same:\n%s", got)
	}
	again, addr2, err := programblocks.Decode(bin, addr)
	if err != nil || addr2 != addr || !bytes.Equal(again.Encode(), bin) {
		t.Fatalf("binary form does not round trip: %v", err)
	}
	// The signature is not part of the address, so a fresh build with the
	// same signer and date has the address of the committed example.
	if m.Address() != addr {
		t.Fatalf("manifest.ini is stale, run go test -update: address %s, built %s", addr, m.Address())
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
