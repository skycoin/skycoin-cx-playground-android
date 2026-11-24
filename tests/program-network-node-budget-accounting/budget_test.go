// Package programnetworknodebudgetaccounting checks how a medium node
// counts its budget: the bytes it counts are the manifest and packing
// bytes on disk, haves that arrive together cannot overcommit the budget
// because a download reserves its bytes when it starts, a pinned program
// is kept past the budget (D38), and a restart counts the same bytes from
// the store.
package programnetworknodebudgetaccounting

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

func startNode(t *testing.T, name, dir string, trusted cipher.PubKey, settings programnetwork.Settings) *programnetwork.Node {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
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

func TestMediumBudgetAccounting(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program network budget"))
	apps := []string{"bench-fib", "bench-loops", "bench-alloc", "bench-classic", "triangles", "benchmarks"}
	var progs []map[string][]byte
	var addrs []programblocks.Hash
	var sizes []int64
	var total int64
	for _, app := range apps {
		files, err := programblocks.ReadProgramDirectory("../../apps/" + app)
		if err != nil {
			t.Fatal(err)
		}
		a, s := plan(t, files, secret)
		progs, addrs, sizes, total = append(progs, files), append(addrs, a), append(sizes, s), total+s
	}
	// The budget holds about half the bytes of the small programs, and the
	// largest program, benchmarks, is pinned and over the budget alone.
	largest := len(apps) - 1
	budget := (total - sizes[largest]) / 2
	if sizes[largest] <= budget {
		t.Fatalf("sizes %v: the pinned program fits the budget %d", sizes, budget)
	}
	pinsFile := filepath.Join(t.TempDir(), "pins.txt")
	if err := programnetwork.WritePins(pinsFile, []string{addrs[largest].String()[:12]}); err != nil {
		t.Fatal(err)
	}
	settings := programnetwork.Settings{Type: programnetwork.TypeMedium, Budget: budget, PinsFile: pinsFile}
	publisher := startNode(t, "publisher", t.TempDir(), pub, programnetwork.Settings{Type: programnetwork.TypeSuper})
	mediumDir := t.TempDir()
	medium := startNode(t, "medium", mediumDir, pub, settings)
	if err := medium.Subscribe(publisher.Addr().String()); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "subscription", 5*time.Second, func() bool { return publisher.Stats().Subscribers == 1 })
	// Publish them all at once, so the haves arrive together.
	for _, files := range progs {
		go publisher.Publish(files, secret, publishDate)
	}
	waitFor(t, "every have", 10*time.Second, func() bool { return medium.Stats().HavesReceived == len(apps) })
	waitFor(t, "downloads", 10*time.Second, func() bool {
		s := medium.Stats()
		return s.Downloads+s.HavesIgnored == len(apps)
	})
	time.Sleep(300 * time.Millisecond)
	s := medium.Stats()
	var unpinned, expect int64
	held := 0
	for i, a := range addrs {
		if medium.Holds(a) {
			held++
			expect += sizes[i]
			if i != largest {
				unpinned += sizes[i]
			}
		}
	}
	if !medium.Holds(addrs[largest]) {
		t.Error("the pinned program over the budget is not held")
	}
	if unpinned > budget {
		t.Errorf("unpinned programs take %d bytes, over the budget %d", unpinned, budget)
	}
	if s.StoredBytes != expect || diskBytes(t, mediumDir) != expect {
		t.Errorf("counted %d bytes, %d on disk, held programs take %d", s.StoredBytes, diskBytes(t, mediumDir), expect)
	}
	t.Logf("sizes %v, budget %d: holds %d programs, %d bytes, %d unpinned within the budget, pinned %d over it, %d haves ignored",
		sizes, budget, held, s.StoredBytes, unpinned, sizes[largest], s.HavesIgnored)

	medium.Close()
	restarted := startNode(t, "medium restarted", mediumDir, pub, settings)
	if got := restarted.Stats().StoredBytes; got != expect {
		t.Errorf("restarted medium counts %d bytes, want %d", got, expect)
	}
}
