// Package programloadercxmatchesgo serves programs over UDP on the loopback
// and loads each with the CX loader of programloader/loader.cx and with the Go client,
// which must give the same bytes. It also checks the #lz envelope, the
// signer pin and a small packet size, and that the loaded program runs.
package programloadercxmatchesgo

import (
	"bytes"
	"math/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/apprun"
	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programloader"
)

var seed = []byte("program loader cx matches go")

func programs(t testing.TB) map[string]map[string][]byte {
	out := map[string]map[string][]byte{}
	for name, dir := range map[string]string{"fib-and-primes": "../program-manifest-round-trip/program", "benchmarks": "../../apps/benchmarks"} {
		files, err := programblocks.ReadProgramDirectory(dir)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = files
	}
	return out
}

func serve(t *testing.T, catalog *programblocks.Catalog, size int) string {
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	go catalog.Serve(server, size)
	return server.LocalAddr().String()
}

func load(t testing.TB, args ...string) (map[string][]byte, string, error) {
	return loadWith(cxrun.Options{}, args...)
}

func loadWith(opts cxrun.Options, args ...string) (map[string][]byte, string, error) {
	var out bytes.Buffer
	files, err := programloader.Load(map[string][]byte{programloader.Entry: programloader.Source}, programloader.Entry, args, opts, &out)
	return files, out.String(), err
}

func TestLoaderMatchesGoClient(t *testing.T) {
	all := programs(t)
	pub, secret := cipher.MustGenerateDeterministicKeyPair(seed)
	catalog, err := programblocks.NewCatalog(all, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	addr := serve(t, catalog, programblocks.DefaultPacketSize)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := programblocks.NewClient(conn, pub)
	for name := range all {
		files, out, err := load(t, addr, name, "signer="+pub.Hex())
		if err != nil {
			t.Fatalf("%s: %v\n%s", name, err, out)
		}
		t.Logf("%s:\n%s", name, out)
		p, err := c.FetchProgram(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != len(p.Files) {
			t.Fatalf("%s: CX loaded %d files, Go %d", name, len(files), len(p.Files))
		}
		for path, b := range p.Files {
			if !bytes.Equal(files[path], b) {
				t.Fatalf("%s: %s differs between the CX loader and the Go client", name, path)
			}
		}
	}

	// The loaded program runs.
	files, out, err := load(t, addr, "fib-and-primes", "trusted="+pub.Hex())
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	app, err := programblocks.AppManifest(files)
	if err != nil {
		t.Fatal(err)
	}
	var run bytes.Buffer
	if err := apprun.Run(app, files, cxrun.Options{}, nil, &run); err != nil || !strings.Contains(run.String(), "primes below 100: 25") {
		t.Fatalf("loaded program: %v\n%s", err, run.String())
	}
}

func TestLoaderRefuses(t *testing.T) {
	_, secret := cipher.MustGenerateDeterministicKeyPair(seed)
	catalog, err := programblocks.NewCatalog(programs(t), secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	addr := serve(t, catalog, programblocks.DefaultPacketSize)
	other, _ := cipher.MustGenerateDeterministicKeyPair([]byte("someone else"))
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{addr, "fib-and-primes", "signer=" + other.Hex()}, "not by the pinned signer"},
		{[]string{addr, "no-such-program"}, "the server has no program no-such-program"},
	} {
		_, out, err := load(t, c.args...)
		if err == nil || !strings.Contains(out, c.want) {
			t.Fatalf("%q: %v, printed %q, want %q", c.args, err, out, c.want)
		}
	}
}

// lossyConn drops a share of the packets the server reads and sends, and
// sends some replies twice.
type lossyConn struct {
	net.PacketConn
	mu      sync.Mutex
	rand    *rand.Rand
	dropped int
}

func (c *lossyConn) roll() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rand.Float64()
}

func (c *lossyConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(b)
		if err != nil || c.roll() >= 0.2 {
			return n, addr, err
		}
		c.mu.Lock()
		c.dropped++
		c.mu.Unlock()
	}
}

func (c *lossyConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	r := c.roll()
	if r < 0.2 {
		c.mu.Lock()
		c.dropped++
		c.mu.Unlock()
		return len(b), nil
	}
	if r > 0.9 {
		c.PacketConn.WriteTo(b, addr)
	}
	return c.PacketConn.WriteTo(b, addr)
}

// TestLoaderForms loads with #lz envelopes, at 128-byte packets and through
// a link that drops a fifth of the packets each way.
func TestLoaderForms(t *testing.T) {
	all := programs(t)
	pub, secret := cipher.MustGenerateDeterministicKeyPair(seed)
	plain, err := programblocks.NewCatalog(all, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	compressed, err := programblocks.NewCompressedCatalog(all, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	lossy, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lossy.Close()
	go plain.Serve(&lossyConn{PacketConn: lossy, rand: rand.New(rand.NewSource(1))}, programblocks.DefaultPacketSize)
	for _, c := range []struct {
		name string
		args []string
	}{
		{"#lz", []string{serve(t, compressed, programblocks.DefaultPacketSize), "benchmarks"}},
		{"128-byte packets", []string{serve(t, plain, 128), "benchmarks", "packet=128"}},
		{"lossy link", []string{lossy.LocalAddr().String(), "benchmarks", "wait=50", "tries=30"}},
	} {
		files, out, err := load(t, append(c.args, "trusted="+pub.Hex())...)
		if err != nil {
			t.Fatalf("%s: %v\n%s", c.name, err, out)
		}
		t.Logf("%s:\n%s", c.name, out)
		for path, b := range all["benchmarks"] {
			if !bytes.Equal(files[path], b) {
				t.Fatalf("%s: %s differs", c.name, path)
			}
		}
	}
}

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

// TestLoaderSignerTrust checks the four cases of D27 in the CX loader: an
// invalid signature is refused, a trusted key runs without a question, and
// any other valid key runs only when the runner confirms it.
func TestLoaderSignerTrust(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair(seed)
	catalog, err := programblocks.NewCatalog(programs(t), secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	addr := serve(t, catalog, programblocks.DefaultPacketSize)
	bad, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	go catalog.Serve(badSignatureConn{bad}, programblocks.DefaultPacketSize)
	other, _ := cipher.MustGenerateDeterministicKeyPair([]byte("someone else"))

	for _, c := range []struct {
		name    string
		server  string
		trusted cipher.PubKey
		confirm bool
		asked   bool   // the runner is asked about the signer
		want    string // in the output of a refused load, "" when it loads
	}{
		{"invalid signature", bad.LocalAddr().String(), pub, true, false, "envelope signature does not match its hash"},
		{"trusted key", addr, pub, false, false, ""},
		{"other key, confirmed", addr, other, true, true, ""},
		{"other key, refused", addr, other, false, true, "the user did not confirm a program signed by " + pub.Hex()},
	} {
		var asked []string
		opts := cxrun.Options{ProgramSigner: func(key string, trusted bool) bool {
			if trusted {
				return true
			}
			asked = append(asked, key)
			return c.confirm
		}}
		files, out, err := loadWith(opts, c.server, "fib-and-primes", "trusted="+c.trusted.Hex())
		if c.want == "" && (err != nil || len(files) == 0) {
			t.Fatalf("%s: %v\n%s", c.name, err, out)
		}
		if c.want != "" && (err == nil || !strings.Contains(out, c.want)) {
			t.Fatalf("%s: %v, printed %q, want %q", c.name, err, out, c.want)
		}
		if wantAsked := c.asked; (len(asked) == 1 && asked[0] == pub.Hex()) != wantAsked || len(asked) > 1 {
			t.Fatalf("%s: asked about %q, want a question %t", c.name, asked, wantAsked)
		}
		if !strings.Contains(out, "signer "+pub.Hex()) && c.want != "envelope signature does not match its hash" {
			t.Fatalf("%s: the key is not shown\n%s", c.name, out)
		}
		t.Logf("%s:\n%s", c.name, out)
	}

	// With no ProgramSigner the runner refuses a key that is not trusted.
	if _, out, err := load(t, addr, "fib-and-primes"); err == nil || !strings.Contains(out, "did not confirm") {
		t.Fatalf("no ProgramSigner: %v\n%s", err, out)
	}
}
