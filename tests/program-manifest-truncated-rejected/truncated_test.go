// Package programmanifesttruncated checks that every proper prefix of a
// manifest envelope is rejected, and so is an envelope with bytes added at
// its end and a text form cut at any line.
package programmanifesttruncated

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func TestTruncatedManifestRejected(t *testing.T) {
	envelope, err := os.ReadFile("../program-manifest-round-trip/manifest.txt")
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := programblocks.Decode(envelope, programblocks.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	header, err := programblocks.HeaderLength(envelope)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < len(envelope); n++ {
		_, _, err := programblocks.Decode(envelope[:n], programblocks.Hash{})
		if err == nil {
			t.Fatalf("prefix of %d of %d bytes accepted", n, len(envelope))
		}
		if n >= header && !errors.Is(err, programblocks.ErrTruncated) {
			t.Fatalf("prefix of %d bytes: %v, want ErrTruncated", n, err)
		}
	}
	if _, _, err := programblocks.Decode(append(envelope, 0), programblocks.Hash{}); err == nil {
		t.Fatal("envelope with an extra byte accepted")
	}
	// Cutting the text at a line end drops whole entries or sections.
	text := m.EncodeText()
	if !bytes.HasSuffix(envelope, text) {
		t.Fatal("the example envelope does not end with its text form")
	}
	for n := 0; n < len(text); n++ {
		if text[n] != '\n' {
			continue
		}
		if _, err := programblocks.DecodeText(text[:n+1]); err == nil && n+1 < len(text) {
			t.Fatalf("text prefix of %d of %d bytes accepted", n+1, len(text))
		}
	}
}
