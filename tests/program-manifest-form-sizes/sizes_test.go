// Package programmanifestformsizes measures the binary and text forms of a
// real app manifest, raw and compressed, and how long each takes to parse.
// Run with -v to see the table that decision D30 quotes.
package programmanifestformsizes

import (
	"bytes"
	"compress/gzip"
	"math/rand"
	"testing"
	"time"

	"github.com/pierrec/lz4/v4"
	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func gzipLength(b []byte) int {
	var out bytes.Buffer
	w, _ := gzip.NewWriterLevel(&out, gzip.BestCompression)
	w.Write(b)
	w.Close()
	return out.Len()
}

func lz4Length(b []byte) int {
	out := make([]byte, lz4.CompressBlockBound(len(b)))
	n, _ := lz4.CompressBlock(b, out, nil)
	return n
}

func signed(t testing.TB, files map[string][]byte) *programblocks.Manifest {
	_, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program manifest form sizes"))
	m, _, err := programblocks.Build(files, programblocks.DefaultBlockSize, time.Unix(1795478400, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Sign(secret); err != nil {
		t.Fatal(err)
	}
	return m
}

func benchmarkApp(t testing.TB) map[string][]byte {
	fsys, _, err := manifest.ReadDir("../../apps/benchmarks")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for p, f := range fsys {
		files[p] = f.Data
	}
	return files
}

// program64KB is a 64 KB serialized-program sized file plus its metadata.
func program64KB() map[string][]byte {
	b := make([]byte, 64<<10)
	rand.New(rand.NewSource(1)).Read(b)
	return map[string][]byte{"cxapp.txt": []byte("[app]\nname = 64 KB program\nform = ir\nentry = main.cxir\n"), "main.cxir": b}
}

func TestFormSizes(t *testing.T) {
	for _, c := range []struct {
		name  string
		files map[string][]byte
	}{{"apps/benchmarks", benchmarkApp(t)}, {"64 KB program", program64KB()}} {
		m := signed(t, c.files)
		bin, text := m.Encode(), m.EncodeText()
		t.Logf("%-16s blocks %4d  binary %6d gzip %6d lz4 %6d  text %6d gzip %6d lz4 %6d",
			c.name, len(m.Blocks), len(bin), gzipLength(bin), lz4Length(bin), len(text), gzipLength(text), lz4Length(text))
		_, b2, _, err := programblocks.DecodeText(text)
		if err != nil || !bytes.Equal(b2, bin) {
			t.Fatalf("%s: text form does not round trip: %v", c.name, err)
		}
	}
}

func BenchmarkDecodeBinary(b *testing.B) {
	bin := signed(b, program64KB()).Encode()
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := programblocks.Decode(bin, programblocks.Hash{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeText(b *testing.B) {
	text := signed(b, program64KB()).EncodeText()
	b.ReportAllocs()
	for b.Loop() {
		if _, _, _, err := programblocks.DecodeText(text); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSignatureVerify is the part both forms share, for scale.
func BenchmarkSignatureVerify(b *testing.B) {
	bin := signed(b, program64KB()).Encode()
	for b.Loop() {
		programblocks.ParseHeader(bin)
	}
}
