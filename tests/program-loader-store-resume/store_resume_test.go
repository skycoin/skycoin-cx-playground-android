// Test program-loader-store-resume syncs the manifest list of a server into
// a store, stops a download part way, opens the store again and resumes:
// the second download asks only for the missing blocks and the files arrive
// whole. It also checks the trusted signer file and that a stranger's
// manifest list is refused.
package storeresume

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// stoppingConn fails every read after the first reads, as a dropped
// network does.
type stoppingConn struct {
	net.Conn
	reads int
}

var errStopped = errors.New("the download was stopped")

func (c *stoppingConn) Read(b []byte) (int, error) {
	if c.reads == 0 {
		return 0, errStopped
	}
	c.reads--
	return c.Conn.Read(b)
}

func serve(t *testing.T, seed string) (cipher.PubKey, string, map[string]map[string][]byte) {
	t.Helper()
	programs := map[string]map[string][]byte{}
	for name, dir := range map[string]string{"fib-and-primes": "../program-manifest-round-trip/program", "benchmarks": "../../apps/benchmarks"} {
		files, err := programblocks.ReadProgramDirectory(dir)
		if err != nil {
			t.Fatal(err)
		}
		programs[name] = files
	}
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte(seed))
	catalog, err := programblocks.NewCatalog(programs, secret, time.Unix(1795400000, 0))
	if err != nil {
		t.Fatal(err)
	}
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	go catalog.Serve(server, programblocks.SmallPacketSize)
	return pub, server.LocalAddr().String(), programs
}

func client(t *testing.T, addr string) *programblocks.Client {
	t.Helper()
	conn, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	c := programblocks.NewClient(conn)
	c.PacketSize = programblocks.SmallPacketSize
	return c
}

func TestResumeAfterStoppedDownload(t *testing.T) {
	pub, addr, programs := serve(t, "program loader store")
	dir := filepath.Join(t.TempDir(), "programs")
	s, err := programstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(client(t, addr)); err == nil {
		t.Fatal("synced with no trusted signer")
	}
	if err := s.TrustSigner(pub, "test server"); err != nil {
		t.Fatal(err)
	}
	if err := s.TrustSigner(pub, "again"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(s.TrustedPath()); string(b) != pub.Hex()+" test server\n" {
		t.Fatalf("trusted.txt %q", b)
	}
	rows, err := s.Sync(client(t, addr))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Name != "benchmarks" || rows[0].State != programstore.StateRemote || rows[1].Name != "fib-and-primes" {
		t.Fatalf("rows %+v", rows)
	}
	target, err := programblocks.HashFromHex(rows[0].Address)
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Manifest(target)
	if err != nil {
		t.Fatal(err)
	}
	total := len(m.Blocks)

	stopping := client(t, addr)
	stopping.Conn = &stoppingConn{Conn: stopping.Conn, reads: 4}
	if _, err := s.Download(stopping, target); !errors.Is(err, errStopped) {
		t.Fatalf("stopped download: %v", err)
	}
	listed, err := s.ReadIndex()
	if err != nil || listed[0].State != programstore.StatePartial {
		t.Fatalf("after the stop: %v %+v", err, listed)
	}

	reopened, err := programstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := reopened.OpenPacking(target, m)
	if err != nil {
		t.Fatal(err)
	}
	kept := total - p.Assembly.MissingBlocks()
	p.Close()
	if kept == 0 || kept == total {
		t.Fatalf("%d of %d blocks kept after the stop", kept, total)
	}

	resume := client(t, addr)
	files, err := reopened.Download(resume, target)
	if err != nil {
		t.Fatal(err)
	}
	for path, b := range programs["benchmarks"] {
		if !bytes.Equal(files[path], b) {
			t.Fatalf("%s differs after the resume", path)
		}
	}
	full := client(t, addr)
	if _, err := full.FetchFiles(target, m); err != nil {
		t.Fatal(err)
	}
	if resume.Stats.BytesReceived >= full.Stats.BytesReceived {
		t.Fatalf("resume received %d bytes, a full download %d", resume.Stats.BytesReceived, full.Stats.BytesReceived)
	}
	t.Logf("%d blocks, %d kept after the stop; the resume received %d bytes in %d requests, a full download %d bytes in %d requests",
		total, kept, resume.Stats.BytesReceived, resume.Stats.RequestsSent, full.Stats.BytesReceived, full.Stats.RequestsSent)
	listed, err = reopened.ReadIndex()
	if err != nil || listed[0].State != programstore.StateCached {
		t.Fatalf("after the resume: %v %+v", err, listed)
	}

	// A list signed by a stranger is refused, and the kept list stays.
	stranger, _ := serveStranger(t)
	if _, err := reopened.Sync(client(t, stranger)); !errors.Is(err, programblocks.ErrUntrusted) {
		t.Fatalf("stranger's list: %v", err)
	}
	if _, listAddr, err := reopened.List(); err != nil || listAddr.String() == "" {
		t.Fatalf("kept list: %v", err)
	}
}

func serveStranger(t *testing.T) (string, cipher.PubKey) {
	pub, addr, _ := serve(t, "someone else")
	return addr, pub
}
