// Package programnetworknodetypesmixed floods programs through a network
// of every node type and checks what each keeps: the super node holds all,
// a medium node holds only its pinned program, another only what fits its
// budget, a super node behind the pinned medium node gets only what that
// node holds, and the thin node holds nothing yet fetches a program on
// demand from the super node.
package programnetworknodetypesmixed

import (
	"bytes"
	"net"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// publishDate is fixed, so a program's address is known before it is
// published: the address hashes the files, the date and the signer key.
var publishDate = time.Unix(1700000000, 0)

func readPrograms(t *testing.T) []map[string][]byte {
	var out []map[string][]byte
	for _, app := range []string{"benchmarks", "bench-fib", "triangles"} {
		files, err := programblocks.ReadProgramDirectory("../../apps/" + app)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, files)
	}
	return out
}

// plan returns the address a program will have and the bytes it takes in
// a store, its signed manifest and its packing.
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

func startNode(t *testing.T, name string, trusted cipher.PubKey, settings programnetwork.Settings) *programnetwork.Node {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	st, err := programstore.Open(t.TempDir())
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

func subscribe(t *testing.T, n, to *programnetwork.Node) {
	t.Helper()
	if err := n.Subscribe(to.Addr().String()); err != nil {
		t.Fatal(err)
	}
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

// heldIndexes returns the indexes of addrs that n holds.
func heldIndexes(n *programnetwork.Node, addrs []programblocks.Hash) []int {
	out := []int{}
	for i, a := range addrs {
		if n.Holds(a) {
			out = append(out, i)
		}
	}
	return out
}

func TestMixedNodeTypes(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program network node types"))
	progs := readPrograms(t)
	addrs := make([]programblocks.Hash, len(progs))
	sizes := make([]int64, len(progs))
	for i, p := range progs {
		addrs[i], sizes[i] = plan(t, p, secret)
	}
	sorted := slices.Clone(sizes)
	slices.Sort(sorted)
	// The budget is the middle size: it fits either of the two smaller
	// programs alone, never both and never the largest, so the budget node
	// keeps exactly one, whichever arrives first.
	budget := sorted[1]
	if sorted[2] == budget {
		t.Fatalf("program sizes %v: two share the largest size", sizes)
	}
	pinsFile := filepath.Join(t.TempDir(), "pins.txt")
	if err := programnetwork.WritePins(pinsFile, []string{addrs[1].String()[:16]}); err != nil {
		t.Fatal(err)
	}

	publisher := startNode(t, "publisher", pub, programnetwork.Settings{Type: programnetwork.TypeSuper})
	super := startNode(t, "super", pub, programnetwork.Settings{Type: programnetwork.TypeSuper})
	pinned := startNode(t, "pinned", pub, programnetwork.Settings{Type: programnetwork.TypeMedium, PinsFile: pinsFile})
	budgeted := startNode(t, "budgeted", pub, programnetwork.Settings{Type: programnetwork.TypeMedium, Budget: budget})
	thin := startNode(t, "thin", pub, programnetwork.Settings{Type: programnetwork.TypeThin})
	behindPinned := startNode(t, "behind-pinned", pub, programnetwork.Settings{Type: programnetwork.TypeSuper})
	subscribe(t, super, publisher)
	for _, n := range []*programnetwork.Node{pinned, budgeted, thin} {
		subscribe(t, n, super)
	}
	subscribe(t, behindPinned, pinned)
	// A node that subscribes to the thin node gets no lease.
	subscribe(t, behindPinned, thin)
	waitFor(t, "subscriptions", 5*time.Second, func() bool {
		return publisher.Stats().Subscribers == 1 && super.Stats().Subscribers == 3 && pinned.Stats().Subscribers == 1
	})

	for i, files := range progs {
		addr, err := publisher.Publish(files, secret, publishDate)
		if err != nil {
			t.Fatal(err)
		}
		if addr != addrs[i] {
			t.Fatalf("program %d published at %s, planned %s", i, addr, addrs[i])
		}
	}
	waitFor(t, "the flood", 10*time.Second, func() bool {
		return len(heldIndexes(super, addrs)) == 3 && len(heldIndexes(pinned, addrs)) == 1 &&
			len(heldIndexes(budgeted, addrs)) == 1 && len(heldIndexes(behindPinned, addrs)) == 1 && len(thin.Known()) == 3
	})
	time.Sleep(500 * time.Millisecond)

	if got := heldIndexes(super, addrs); len(got) != 3 {
		t.Errorf("super holds %v, want all", got)
	}
	if got := heldIndexes(pinned, addrs); !slices.Equal(got, []int{1}) {
		t.Errorf("pinned medium holds %v, want [1]", got)
	}
	if got := heldIndexes(behindPinned, addrs); !slices.Equal(got, []int{1}) {
		t.Errorf("the super node behind the pinned medium holds %v, want [1], only what that node holds", got)
	}
	if s := budgeted.Stats(); len(heldIndexes(budgeted, addrs)) != 1 || s.StoredBytes > budget || s.HavesIgnored != 2 {
		t.Errorf("budget medium holds %v, %d of %d bytes, %d haves ignored: %+v", heldIndexes(budgeted, addrs), s.StoredBytes, budget, s.HavesIgnored, s)
	}
	if s := pinned.Stats(); s.HavesIgnored != 2 || s.Downloads != 1 {
		t.Errorf("pinned medium: %+v, want 2 haves ignored and 1 download", s)
	}
	ts := thin.Stats()
	if got := heldIndexes(thin, addrs); len(got) != 0 || ts.Downloads != 0 || ts.HavesIgnored != 3 || ts.Subscribers != 0 {
		t.Errorf("thin holds %v: %+v, want nothing held, no download, 3 haves ignored, no subscriber", got, ts)
	}
	t.Logf("program sizes %v, budget %d: super holds %v, pinned medium %v, budget medium %v (%d bytes), behind pinned %v, thin %v and knows %d",
		sizes, budget, heldIndexes(super, addrs), heldIndexes(pinned, addrs), heldIndexes(budgeted, addrs), budgeted.Stats().StoredBytes,
		heldIndexes(behindPinned, addrs), heldIndexes(thin, addrs), len(thin.Known()))

	// The thin node fetches on demand from the super node that announced
	// the program, keeps it for itself, and still neither holds nor serves.
	start := time.Now()
	p, err := thin.Fetch(addrs[2])
	if err != nil {
		t.Fatal(err)
	}
	sameFiles(t, p.Files, progs[2])
	if thin.Holds(addrs[2]) || thin.Stats().Downloads != 1 || thin.Stats().HavesSent != 0 {
		t.Errorf("thin after a fetch: holds %v, %+v", thin.Holds(addrs[2]), thin.Stats())
	}
	t.Logf("thin fetched %s from %s in %d ms", p.Name, p.From, time.Since(start).Milliseconds())
	again, err := thin.Fetch(addrs[2])
	if err != nil || again.From != "" || thin.Stats().Downloads != 1 {
		t.Errorf("a second fetch should read the thin node's own store: from %q, %+v, %v", again.From, thin.Stats(), err)
	}
	conn, err := net.Dial("udp", thin.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	c := programblocks.NewClient(conn, pub)
	c.MaxTries = 2
	if _, _, err := c.FetchManifest(addrs[2]); err == nil {
		t.Error("the thin node served a manifest")
	}
	if _, err := thin.Publish(progs[0], secret, publishDate); err == nil {
		t.Error("the thin node published")
	}
}

// sameFiles fails unless got holds every file of want, byte for byte.
func sameFiles(t *testing.T, got, want map[string][]byte) {
	t.Helper()
	for path, b := range want {
		if !bytes.Equal(got[path], b) {
			t.Fatalf("%s differs", path)
		}
	}
}
