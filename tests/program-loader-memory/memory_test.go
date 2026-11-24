// Test program-loader-memory keeps downloaded programs in RAM: load,
// unload and reload give the same files, a limit of one keeps only the
// last program, as on an ESP32, and a program cleared from the store can
// no longer load.
package memory

import (
	"bytes"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/apprun"
	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

func TestLoadUnloadReload(t *testing.T) {
	programs := map[string]map[string][]byte{}
	for name, dir := range map[string]string{"fib-and-primes": "../program-manifest-round-trip/program", "benchmarks": "../../apps/benchmarks"} {
		files, err := programblocks.ReadProgramDirectory(dir)
		if err != nil {
			t.Fatal(err)
		}
		programs[name] = files
	}
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program loader memory"))
	catalog, err := programblocks.NewCatalog(programs, secret, time.Unix(1795400000, 0))
	if err != nil {
		t.Fatal(err)
	}
	server, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go catalog.Serve(server, programblocks.DefaultPacketSize)
	conn, err := net.Dial("udp", server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := programblocks.NewClient(conn)

	s, err := programstore.Open(filepath.Join(t.TempDir(), "programs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TrustSigner(pub, ""); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Sync(c)
	if err != nil {
		t.Fatal(err)
	}
	addr := map[string]programblocks.Hash{}
	for _, r := range rows {
		a, _ := programblocks.HashFromHex(r.Address)
		addr[r.Name] = a
		if _, err := s.Download(c, a); err != nil {
			t.Fatal(err)
		}
	}

	mem := programstore.NewMemory(s, 0)
	first, err := mem.Load(addr["fib-and-primes"])
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := mem.Load(addr["fib-and-primes"]); again != first {
		t.Fatal("a held program was read again")
	}
	var out bytes.Buffer
	if err := apprun.Run(first.App, first.Files, cxrun.Options{}, nil, &out); err != nil || out.Len() == 0 {
		t.Fatalf("run from RAM: %v %q", err, out.String())
	}
	mem.Unload(addr["fib-and-primes"])
	if mem.Held(addr["fib-and-primes"]) != nil || mem.HeldBytes() != 0 {
		t.Fatal("unloaded program still held")
	}
	reloaded, err := mem.Load(addr["fib-and-primes"])
	if err != nil || reloaded == first {
		t.Fatalf("reload: %v", err)
	}
	for path, b := range first.Files {
		if !bytes.Equal(reloaded.Files[path], b) {
			t.Fatalf("%s differs after the reload", path)
		}
	}

	one := programstore.NewMemory(s, 1)
	if _, err := one.Load(addr["fib-and-primes"]); err != nil {
		t.Fatal(err)
	}
	if _, err := one.Load(addr["benchmarks"]); err != nil {
		t.Fatal(err)
	}
	if one.Held(addr["fib-and-primes"]) != nil || one.Held(addr["benchmarks"]) == nil {
		t.Fatal("a limit of one keeps the last program only")
	}
	one.Clear()
	if one.HeldBytes() != 0 {
		t.Fatal("clear left programs held")
	}

	if err := s.Clear(addr["benchmarks"]); err != nil {
		t.Fatal(err)
	}
	if _, err := one.Load(addr["benchmarks"]); err == nil {
		t.Fatal("a program cleared from the store loaded")
	}
	listed, _ := s.ReadIndex()
	for _, r := range listed {
		if r.Name == "benchmarks" && r.State != programstore.StateRemote {
			t.Fatalf("cleared program is %s", r.State)
		}
	}
}
