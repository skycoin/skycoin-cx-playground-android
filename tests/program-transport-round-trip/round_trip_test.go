// Package programtransportroundtrip serves two programs over UDP on the
// loopback and fetches each one at both packet sizes, checking that the
// files arrive unchanged, the program runs, and an untrusted signer or an
// unknown program is refused.
package programtransportroundtrip

import (
	"bytes"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/apprun"
	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func TestRoundTrip(t *testing.T) {
	programs := map[string]map[string][]byte{}
	for name, dir := range map[string]string{"fib-and-primes": "../program-manifest-round-trip/program", "benchmarks": "../../apps/benchmarks"} {
		files, err := programblocks.ReadProgramDirectory(dir)
		if err != nil {
			t.Fatal(err)
		}
		programs[name] = files
	}
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program transport round trip"))
	catalog, err := programblocks.NewCatalog(programs, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{programblocks.DefaultPacketSize, programblocks.SmallPacketSize} {
		server, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go catalog.Serve(server, size)
		conn, err := net.Dial("udp", server.LocalAddr().String())
		if err != nil {
			t.Fatal(err)
		}
		c := programblocks.NewClient(conn, pub)
		c.PacketSize = size
		for name, files := range programs {
			p, err := c.FetchProgram(name)
			if err != nil {
				t.Fatalf("packet %d, %s: %v", size, name, err)
			}
			for path, b := range files {
				if !bytes.Equal(p.Files[path], b) {
					t.Fatalf("packet %d, %s: %s differs", size, name, path)
				}
			}
		}
		if c.Stats.LargestReply > size {
			t.Fatalf("largest reply %d bytes, packet size %d", c.Stats.LargestReply, size)
		}
		t.Logf("packet %d: %d requests, %d replies, largest %d bytes", size, c.Stats.RequestsSent, c.Stats.RepliesAccepted, c.Stats.LargestReply)

		if _, err := c.FetchProgram("no-such-program"); err == nil {
			t.Fatal("unknown program fetched")
		}
		other, _ := cipher.MustGenerateDeterministicKeyPair([]byte("someone else"))
		c.TrustedSigners = []cipher.PubKey{other}
		if _, err := c.FetchProgram("fib-and-primes"); !errors.Is(err, programblocks.ErrUntrusted) {
			t.Fatalf("untrusted signer: %v", err)
		}
		conn.Close()
		server.Close()
	}

	// The fetched program runs and prints what the example expects.
	server, _ := net.ListenPacket("udp", "127.0.0.1:0")
	defer server.Close()
	go catalog.Serve(server, programblocks.DefaultPacketSize)
	conn, _ := net.Dial("udp", server.LocalAddr().String())
	defer conn.Close()
	p, err := programblocks.NewClient(conn, pub).FetchProgram("fib-and-primes")
	if err != nil {
		t.Fatal(err)
	}
	app, err := programblocks.AppManifest(p.Files)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := apprun.Run(app, p.Files, cxrun.Options{}, nil, &out); err != nil {
		t.Fatal(err)
	}
	want, _ := os.ReadFile("../program-manifest-round-trip/expected/stdout.txt")
	if !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("printed %q", out.Bytes())
	}
}
