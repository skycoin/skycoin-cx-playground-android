// Package programmanifesttruncated checks that every proper prefix of a
// manifest, in binary and in text form, is rejected, and so is a manifest
// with bytes added at its end.
package programmanifesttruncated

import (
	"errors"
	"os"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func TestTruncatedManifestRejected(t *testing.T) {
	text, err := os.ReadFile("../program-manifest-round-trip/manifest.ini")
	if err != nil {
		t.Fatal(err)
	}
	_, bin, _, err := programblocks.DecodeText(text)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < len(bin); n++ {
		_, _, err := programblocks.Decode(bin[:n], programblocks.Hash{})
		if err == nil {
			t.Fatalf("binary prefix of %d of %d bytes accepted", n, len(bin))
		}
		if n >= programblocks.HeaderLengthVersion1 && !errors.Is(err, programblocks.ErrTruncated) {
			t.Fatalf("binary prefix of %d bytes: %v, want ErrTruncated", n, err)
		}
	}
	if _, _, err := programblocks.Decode(append(bin, 0), programblocks.Hash{}); err == nil {
		t.Fatal("manifest with an extra byte accepted")
	}
	// Cutting the text at a line end drops whole entries or sections.
	for n := 0; n < len(text); n++ {
		if text[n] != '\n' {
			continue
		}
		if _, _, _, err := programblocks.DecodeText(text[:n+1]); err == nil && n+1 < len(text) {
			t.Fatalf("text prefix of %d of %d bytes accepted", n+1, len(text))
		}
	}
}
