// Package programmanifestformsizes measures the manifest envelope of a real
// app, with its text form plain and as LZ4 (#lz), and how long each takes to
// decode. Run with -v to see the table of "Text form" in
// docs/ProgramDistribution.md.
package programmanifestformsizes

import (
	"bytes"
	"compress/gzip"
	"math/rand"
	"testing"
	"time"

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

func signed(t testing.TB, files map[string][]byte, compressed bool) *programblocks.Manifest {
	_, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program manifest form sizes"))
	m, _, err := programblocks.Build(files, programblocks.DefaultBlockSize, time.Unix(1795478400, 0))
	if err != nil {
		t.Fatal(err)
	}
	m.Compressed = compressed
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
		m := signed(t, c.files, false)
		plain, text := m.Encode(), m.EncodeText()
		lz := signed(t, c.files, true).Encode()
		t.Logf("%-16s blocks %4d  text %6d  envelope %6d  #lz envelope %6d  text gzip %6d",
			c.name, len(m.Blocks), len(text), len(plain), len(lz), gzipLength(text))
		for _, b := range [][]byte{plain, lz} {
			d, _, err := programblocks.Decode(b, programblocks.Hash{})
			if err != nil || !bytes.Equal(d.EncodeText(), text) {
				t.Fatalf("%s: envelope does not round trip: %v", c.name, err)
			}
		}
	}
}

func benchmarkDecode(b *testing.B, compressed bool) {
	envelope := signed(b, program64KB(), compressed).Encode()
	b.ReportAllocs()
	for b.Loop() {
		if _, _, err := programblocks.Decode(envelope, programblocks.Hash{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeText(b *testing.B) { benchmarkDecode(b, false) }
func BenchmarkDecodeLZ4(b *testing.B)  { benchmarkDecode(b, true) }

// BenchmarkSignatureVerify is the envelope header alone, for scale.
func BenchmarkSignatureVerify(b *testing.B) {
	envelope := signed(b, program64KB(), false).Encode()
	for b.Loop() {
		programblocks.ParseHeader(envelope)
	}
}
