// Package programnetworksuperfullnoeviction fills a super node's storage
// and checks that it stops taking programs without evicting any, keeps
// serving what it holds, and takes a program again only after one is
// removed by hand with programstore.Store.Remove, as `programnetwork store
// remove` does.
package programnetworksuperfullnoeviction

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

var publishDate = time.Unix(1700000000, 0)

func plan(t *testing.T, files map[string][]byte, secret cipher.SecKey) (programblocks.Hash, int64) {
	m, _, err := programblocks.Build(files, programblocks.DefaultBlockSize, publishDate)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Sign(secret); err != nil {
		t.Fatal(err)
	}
	return m.Address(), int64(len(m.Encode())) + int64(m.PackLength)
}

func startNode(t *testing.T, name, dir, addr string, trusted cipher.PubKey, settings programnetwork.Settings) *programnetwork.Node {
	t.Helper()
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	st, err := programstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	n, err := programnetwork.NewTypedNode(pc, st, []cipher.PubKey{trusted}, settings)
	if err != nil {
		t.Fatal(err)
	}
	n.Logf = func(format string, args ...any) { t.Logf(name+": "+format, args...) }
	go n.Run()
	t.Cleanup(func() { n.Close() })
	return n
}

func waitFor(t *testing.T, what string, limit time.Duration, done func() bool) {
	t.Helper()
	start := time.Now()
	for !done() {
		if time.Since(start) > limit {
			t.Fatalf("%s: not after %v", what, limit)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// diskBytes sums the manifests and packings in a store directory.
func diskBytes(t *testing.T, dir string) int64 {
	var total int64
	for _, sub := range []string{"m", "p"} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				t.Fatal(err)
			}
			total += info.Size()
		}
	}
	return total
}

func TestSuperStopsWhenFull(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program network super full"))
	var progs []map[string][]byte
	var addrs []programblocks.Hash
	var sizes []int64
	// Two small programs fill the capacity, the third does not fit.
	for _, app := range []string{"bench-fib", "triangles", "benchmarks"} {
		files, err := programblocks.ReadProgramDirectory("../../apps/" + app)
		if err != nil {
			t.Fatal(err)
		}
		a, s := plan(t, files, secret)
		progs, addrs, sizes = append(progs, files), append(addrs, a), append(sizes, s)
	}
	capacity := sizes[0] + sizes[1]
	publisher := startNode(t, "publisher", t.TempDir(), "127.0.0.1:0", pub, programnetwork.Settings{Type: programnetwork.TypeSuper})
	superDir := t.TempDir()
	super := startNode(t, "super", superDir, "127.0.0.1:0", pub, programnetwork.Settings{Type: programnetwork.TypeSuper, Budget: capacity})
	if err := super.Subscribe(publisher.Addr().String()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "subscription", 5*time.Second, func() bool { return publisher.Stats().Subscribers == 1 })
	for i, files := range progs {
		if _, err := publisher.Publish(files, secret, publishDate); err != nil {
			t.Fatal(err)
		}
		if i < 2 {
			waitFor(t, "program held", 5*time.Second, func() bool { return super.Holds(addrs[i]) })
		}
	}
	waitFor(t, "refusal", 5*time.Second, func() bool { return super.Stats().HavesRefusedFull == 1 })
	time.Sleep(300 * time.Millisecond)
	s := super.Stats()
	if !super.Holds(addrs[0]) || !super.Holds(addrs[1]) || super.Holds(addrs[2]) {
		t.Fatalf("full super holds %v %v %v, want the first two kept and the third refused", super.Holds(addrs[0]), super.Holds(addrs[1]), super.Holds(addrs[2]))
	}
	if s.StoredBytes != capacity || diskBytes(t, superDir) != capacity || s.Downloads != 2 {
		t.Errorf("full super: %d bytes counted, %d on disk, capacity %d, %d downloads", s.StoredBytes, diskBytes(t, superDir), capacity, s.Downloads)
	}
	t.Logf("sizes %v, capacity %d: holds 2 programs, %d bytes, %d have refused for full storage", sizes, capacity, s.StoredBytes, s.HavesRefusedFull)

	// It keeps serving what it holds.
	conn, err := net.Dial("udp", super.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := programblocks.NewClient(conn, pub)
	m, _, err := c.FetchManifest(addrs[0])
	if err != nil {
		t.Fatalf("full super does not serve a held manifest: %v", err)
	}
	if _, err := c.FetchFiles(addrs[0], m); err != nil {
		t.Fatalf("full super does not serve a held program: %v", err)
	}

	// Removal is manual: remove a program and restart the node on the same
	// port, which then takes the next have of a program that fits. Its
	// fresh subscription ends the old one first, so the publisher sends the
	// catch-up haves of D34 again.
	superAddr := super.Addr().String()
	super.Close()
	st, err := programstore.Open(superDir)
	if err != nil {
		t.Fatal(err)
	}
	row, err := st.FindRow(addrs[0].String()[:16])
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Remove(addrs[0]); err != nil {
		t.Fatal(err)
	}
	if rows, _ := st.ReadIndex(); len(rows) != 1 || rows[0].Number != 1 {
		t.Fatalf("index after removing %s: %+v", row.Name, rows)
	}
	super = startNode(t, "super restarted", superDir, superAddr, pub, programnetwork.Settings{Type: programnetwork.TypeSuper, Budget: capacity})
	if s := super.Stats(); s.StoredBytes != sizes[1] {
		t.Fatalf("after removing %s: %d bytes held, want %d", row.Name, s.StoredBytes, sizes[1])
	}
	waitFor(t, "the removed program taken again", 5*time.Second, func() bool { return super.Holds(addrs[0]) })
	time.Sleep(300 * time.Millisecond)
	if s := super.Stats(); super.Holds(addrs[2]) || s.HavesRefusedFull != 1 || s.StoredBytes != capacity {
		t.Errorf("after the restart: holds the large program %v, %+v", super.Holds(addrs[2]), s)
	}
	t.Logf("removed %s by hand, restarted: took it again from the catch-up haves, refused the large one, %d bytes", row.Name, super.Stats().StoredBytes)
}
