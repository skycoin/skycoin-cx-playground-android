package programloadercxmatchesgo

import (
	"net"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// benchmarkServe serves the programs of the tests on the loopback.
func benchmarkServe(b *testing.B) (string, cipher.PubKey) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair(seed)
	catalog, err := programblocks.NewCatalog(programs(b), secret, time.Now())
	if err != nil {
		b.Fatal(err)
	}
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { server.Close() })
	go catalog.Serve(server, programblocks.DefaultPacketSize)
	return server.LocalAddr().String(), pub
}

// BenchmarkLoaderCX is the loader path: loader.cx lists, fetches and
// verifies apps/benchmarks over the loopback with cipher.HashOfBytes.
func BenchmarkLoaderCX(b *testing.B) {
	addr, pub := benchmarkServe(b)
	for b.Loop() {
		if _, out, err := load(b, addr, "benchmarks", "signer="+pub.Hex()); err != nil {
			b.Fatalf("%v\n%s", err, out)
		}
	}
}

// BenchmarkLoaderGo is the same fetch with the Go client.
func BenchmarkLoaderGo(b *testing.B) {
	addr, pub := benchmarkServe(b)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		b.Fatal(err)
	}
	defer conn.Close()
	c := programblocks.NewClient(conn, pub)
	for b.Loop() {
		if _, err := c.FetchProgram("benchmarks"); err != nil {
			b.Fatal(err)
		}
	}
}
