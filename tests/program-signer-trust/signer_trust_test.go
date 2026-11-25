// Package programsignertrust checks D27 in the Go client: a manifest with
// an invalid signature is refused, a valid signature from any key is
// accepted and its key reported, a trusted key runs without a question,
// and any other key runs only when the user confirms it.
package programsignertrust

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// badSignatureConn makes the recovery byte of every envelope signature the
// server sends invalid, so no key can be recovered from it.
type badSignatureConn struct{ net.PacketConn }

func (c badSignatureConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	b = bytes.Clone(b)
	if i := bytes.Index(b, []byte("CXPM 3\n")); i >= 0 && i+7+130 <= len(b) {
		copy(b[i+7+128:], "ff")
	}
	return c.PacketConn.WriteTo(b, addr)
}

func serve(t *testing.T, wrap func(net.PacketConn) net.PacketConn) (cipher.PubKey, string) {
	files, err := programblocks.ReadProgramDirectory("../program-manifest-round-trip/program")
	if err != nil {
		t.Fatal(err)
	}
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program signer trust"))
	catalog, err := programblocks.NewCatalog(map[string]map[string][]byte{"fib-and-primes": files}, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	go catalog.Serve(wrap(server), programblocks.DefaultPacketSize)
	return pub, server.LocalAddr().String()
}

func fetch(t *testing.T, addr string, pinned ...cipher.PubKey) (*programblocks.FetchedProgram, error) {
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := programblocks.NewClient(conn, pinned...)
	c.MaxTries = 3
	return c.FetchProgram("fib-and-primes")
}

func TestSignerTrust(t *testing.T) {
	pub, addr := serve(t, func(c net.PacketConn) net.PacketConn { return c })
	_, badAddr := serve(t, func(c net.PacketConn) net.PacketConn { return badSignatureConn{c} })
	other, _ := cipher.MustGenerateDeterministicKeyPair([]byte("someone else"))

	// An invalid signature is refused whatever the trust settings.
	if _, err := fetch(t, badAddr); !errors.Is(err, programblocks.ErrSignature) {
		t.Fatalf("invalid signature: %v, want %v", err, programblocks.ErrSignature)
	}

	// A valid signature from a key nobody listed is accepted, and the key
	// it recovers is the server's.
	p, err := fetch(t, addr)
	if err != nil {
		t.Fatal(err)
	}
	if p.Manifest.SignerPublicKey != pub {
		t.Fatalf("signer %s, want %s", p.Manifest.SignerPublicKey.Hex(), pub.Hex())
	}

	// Pinning stays as the strict mode.
	if _, err := fetch(t, addr, other); !errors.Is(err, programblocks.ErrNotPinned) {
		t.Fatalf("pinned to another key: %v", err)
	}
	if _, err := fetch(t, addr, pub); err != nil {
		t.Fatalf("pinned to the signer: %v", err)
	}

	signer := p.Manifest.SignerPublicKey
	for _, c := range []struct {
		name    string
		trusted []cipher.PubKey
		answer  bool
		asked   bool
		runs    bool
	}{
		{"trusted key", []cipher.PubKey{other, pub}, false, false, true},
		{"other key, confirmed", []cipher.PubKey{other}, true, true, true},
		{"other key, refused", []cipher.PubKey{other}, false, true, false},
		{"no trusted keys, refused", nil, false, true, false},
	} {
		asked := false
		err := programblocks.ConfirmSigner(signer, c.trusted, func(k cipher.PubKey) bool {
			asked = k == signer
			return c.answer
		})
		if asked != c.asked || (err == nil) != c.runs {
			t.Fatalf("%s: asked %t, err %v; want asked %t, runs %t", c.name, asked, err, c.asked, c.runs)
		}
		if !c.runs && !errors.Is(err, programblocks.ErrSignerNotConfirmed) {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
	if err := programblocks.ConfirmSigner(signer, nil, nil); !errors.Is(err, programblocks.ErrSignerNotConfirmed) {
		t.Fatalf("nil confirm: %v", err)
	}
}
