// Package programnetworkpeerexchange checks peer exchange as in Bitcoin
// (#41) over UDP: private peers never leave a node and are never learned,
// public peers spread along a chain of nodes with their last-seen times and
// stop spreading once stale, and the limits hold (entries per reply, one
// answer per asker per interval, the size of the peer list).
package programnetworkpeerexchange

import (
	"encoding/binary"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// clock is a settable clock shared by the nodes of a test.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

var start = time.Unix(1795478400, 0).UTC()

func startNode(t *testing.T, c *clock, peers *programnetwork.PeerList) *programnetwork.Node {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	st, err := programstore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n, err := programnetwork.NewTypedNode(pc, st, nil, programnetwork.Settings{Type: programnetwork.TypeMedium})
	if err != nil {
		t.Fatal(err)
	}
	n.Now = c.Now
	if peers != nil {
		n.SetPeers(peers)
	}
	go n.Run()
	t.Cleanup(func() { n.Close() })
	return n
}

func entry(t *testing.T, addr string, seen time.Time, flags byte) programnetwork.PeerEntry {
	a, err := programnetwork.ParsePeerAddress(addr)
	if err != nil {
		t.Fatal(err)
	}
	return programnetwork.PeerEntry{Address: a, LastSeen: seen, Flags: flags}
}

func list(t *testing.T, entries ...programnetwork.PeerEntry) *programnetwork.PeerList {
	l := &programnetwork.PeerList{}
	for _, e := range entries {
		if err := l.SetPeer(e); err != nil {
			t.Fatal(err)
		}
	}
	return l
}

func addresses(l *programnetwork.PeerList) map[string]programnetwork.PeerEntry {
	out := map[string]programnetwork.PeerEntry{}
	for _, e := range l.Peers {
		out[e.Address.String()] = e
	}
	return out
}

func TestPrivatePeersNeverLeave(t *testing.T) {
	c := &clock{now: start}
	a := startNode(t, c, list(t,
		entry(t, "10.1.0.1:7010", start, 0),
		entry(t, "192.168.1.5:7010", start, 0),
		entry(t, "203.0.113.5:7010", start, programnetwork.PeerShareable),
	))
	// b has 203.0.113.9 as a private peer; a reply naming it as public must
	// not make it shareable.
	b := startNode(t, c, list(t, entry(t, "203.0.113.9:7010", start, 0)))
	got, err := b.GetPeers(a.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Address.String() != "203.0.113.5:7010" || !got[0].Shareable() {
		t.Fatalf("a sent %+v, want only its public peer", got)
	}
	learned := addresses(b.Peers())
	if _, ok := learned["10.1.0.1:7010"]; ok {
		t.Fatal("a private peer was learned")
	}
	if e := learned["203.0.113.5:7010"]; !e.Shareable() || !e.LastSeen.Equal(start) {
		t.Fatalf("public peer learned as %+v, want shareable with its last-seen time", e)
	}

	// A crafted reply: one entry not shareable, one that names b's private
	// peer as shareable. Neither changes what b shares.
	fake, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer fake.Close()
	go func() {
		buf := make([]byte, 1500)
		n, from, err := fake.ReadFrom(buf)
		if err != nil || n != 6 {
			return
		}
		r, _ := programnetwork.EncodeGetPeersReply(binary.BigEndian.Uint32(buf[2:]), []programnetwork.PeerEntry{
			entry(t, "198.51.100.1:7010", start, 0),
			entry(t, "203.0.113.9:7010", start, programnetwork.PeerShareable),
		}, 1200)
		fake.WriteTo(r, from)
	}()
	if _, err := b.GetPeers(fake.LocalAddr().String()); err != nil {
		t.Fatal(err)
	}
	learned = addresses(b.Peers())
	if _, ok := learned["198.51.100.1:7010"]; ok {
		t.Error("an entry without the shareable bit was learned")
	}
	if e := learned["203.0.113.9:7010"]; e.Shareable() {
		t.Error("a private peer became shareable from a reply")
	}
	for _, e := range b.Peers().Shareable(c.Now(), netip.AddrPort{}) {
		if e.Address.String() == "203.0.113.9:7010" {
			t.Error("b shares its private peer")
		}
	}
}

func TestPublicPeersSpreadAndGoStale(t *testing.T) {
	c := &clock{now: start}
	seed := startNode(t, c, list(t,
		entry(t, "203.0.113.5:7010", start, programnetwork.PeerShareable),
		entry(t, "[2001:db8::5]:7010", start.Add(-29*24*time.Hour), programnetwork.PeerShareable),
		entry(t, "203.0.113.6:7010", start.Add(-31*24*time.Hour), programnetwork.PeerShareable),
		entry(t, "10.1.0.1:7010", start, 0),
	))
	chain := []*programnetwork.Node{seed}
	for range 4 {
		n := startNode(t, c, nil)
		if _, err := n.GetPeers(chain[len(chain)-1].Addr().String()); err != nil {
			t.Fatal(err)
		}
		chain = append(chain, n)
	}
	last := addresses(chain[len(chain)-1].Peers())
	for _, want := range []string{"203.0.113.5:7010", "[2001:db8::5]:7010"} {
		if e, ok := last[want]; !ok || !e.Shareable() {
			t.Errorf("%s did not reach the end of the chain: %v", want, last)
		}
	}
	if _, ok := last["203.0.113.6:7010"]; ok {
		t.Error("a peer last seen 31 days ago spread")
	}
	if _, ok := last["10.1.0.1:7010"]; ok {
		t.Error("a private peer spread")
	}
	t.Logf("after a chain of %d nodes the last knows %d peers", len(chain), len(last))

	// Two days on, the IPv6 peer is 31 days old: no node gives it out any
	// more, so a new node learns only the fresh one.
	c.add(2*24*time.Hour + programnetwork.PeerAnswerInterval)
	n := startNode(t, c, nil)
	got, err := n.GetPeers(chain[len(chain)-1].Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Address.String() != "203.0.113.5:7010" {
		t.Fatalf("after two days a node got %+v, want only the fresh peer", got)
	}
}

func TestLimits(t *testing.T) {
	c := &clock{now: start}
	l := &programnetwork.PeerList{}
	for i := range 100 {
		if err := l.SetPeer(programnetwork.PeerEntry{Address: netip.AddrPortFrom(netip.AddrFrom4([4]byte{203, 0, 113, byte(i + 1)}), 7010),
			LastSeen: start, Flags: programnetwork.PeerShareable}); err != nil {
			t.Fatal(err)
		}
	}
	server := startNode(t, c, l)
	asker := startNode(t, c, nil)
	got, err := asker.GetPeers(server.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != programnetwork.MaxPeersPerReply {
		t.Fatalf("a reply held %d entries, want %d", len(got), programnetwork.MaxPeersPerReply)
	}
	again, err := asker.GetPeers(server.Addr().String())
	if err != nil || len(again) != 0 {
		t.Fatalf("a second ask within the interval got %d entries, %v, want none", len(again), err)
	}
	other := startNode(t, c, nil)
	if got, err := other.GetPeers(server.Addr().String()); err != nil || len(got) != programnetwork.MaxPeersPerReply {
		t.Fatalf("another asker got %d entries, %v", len(got), err)
	}
	c.add(programnetwork.PeerAnswerInterval)
	if later, err := asker.GetPeers(server.Addr().String()); err != nil || len(later) != programnetwork.MaxPeersPerReply {
		t.Fatalf("after the interval the asker got %d entries, %v", len(later), err)
	}
	t.Logf("100 public peers: %d per reply, none on a second ask within %v", programnetwork.MaxPeersPerReply, programnetwork.PeerAnswerInterval)

	// The list holds at most MaxPeerTableSize entries. Once full it takes a
	// fresher entry in place of the stalest shareable one, and nothing
	// staler.
	full := &programnetwork.PeerList{}
	var batch []programnetwork.PeerEntry
	for i := range programnetwork.MaxPeerTableSize + 50 {
		a := netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 14: byte(i >> 8), 15: byte(i)})
		batch = append(batch, programnetwork.PeerEntry{Address: netip.AddrPortFrom(a, 7010), LastSeen: start.Add(-time.Duration(i) * time.Minute), Flags: programnetwork.PeerShareable})
	}
	if added := full.Learn(batch, start); added != programnetwork.MaxPeerTableSize || len(full.Peers) != programnetwork.MaxPeerTableSize {
		t.Fatalf("learned %d of %d, list of %d, want the list full at %d", added, len(batch), len(full.Peers), programnetwork.MaxPeerTableSize)
	}
	fresh := entry(t, "203.0.113.200:7010", start, programnetwork.PeerShareable)
	if full.Learn([]programnetwork.PeerEntry{fresh}, start) != 1 || len(full.Peers) != programnetwork.MaxPeerTableSize {
		t.Fatal("a full list did not take a fresher entry in place of the stalest")
	}
	stale := entry(t, "203.0.113.201:7010", start.Add(-24*time.Hour), programnetwork.PeerShareable)
	if full.Learn([]programnetwork.PeerEntry{stale}, start) != 0 {
		t.Fatal("a full list took an entry staler than all it holds")
	}
	if err := full.SetPeer(entry(t, "203.0.113.202:7010", start, 0)); err == nil {
		t.Fatal("a full list took a new private peer")
	}
}
