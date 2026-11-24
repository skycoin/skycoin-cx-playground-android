// Package programnetworkpacketsizes checks the have, subscribe and get
// peers packets: each decodes to what was encoded and has the length the
// document gives, a peer entry is 13 bytes for IPv4 and 25 for IPv6, and a
// get peers reply holds at most MaxPeersPerReply entries and never exceeds
// the packet size, also over a real node with many peers.
package programnetworkpacketsizes

import (
	"net"
	"net/netip"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

func TestFixedPackets(t *testing.T) {
	h := programnetwork.Have{Address: programblocks.Sum([]byte("x")), ManifestLength: 2472, PackLength: 1 << 40}
	b := programnetwork.EncodeHave(7, h)
	if len(b) != 50 {
		t.Fatalf("have is %d bytes, want 50", len(b))
	}
	if got, err := programnetwork.DecodeHave(b); err != nil || got != h {
		t.Fatalf("have round trip: %+v %v", got, err)
	}
	ack := programnetwork.EncodeHaveReply(7, h.Address)
	if len(ack) != 38 || ack[1] != 0x90 {
		t.Fatalf("have reply is %d bytes, operation %#x", len(ack), ack[1])
	}
	if got, err := programnetwork.DecodeHaveReply(ack); err != nil || got != h.Address {
		t.Fatalf("have reply round trip: %v %v", got, err)
	}
	for _, p := range [][]byte{programnetwork.EncodeSubscribe(1, 600), programnetwork.EncodeSubscribeReply(1, 600)} {
		if lease, err := programnetwork.DecodeLease(p); len(p) != 10 || err != nil || lease != 600 {
			t.Fatalf("subscribe packet %x: lease %d, %v", p, lease, err)
		}
	}
	g := programnetwork.EncodeGetPeers(9)
	if err := programnetwork.DecodeGetPeers(g); len(g) != 6 || err != nil {
		t.Fatalf("get peers %x: %v", g, err)
	}
	if _, err := programnetwork.DecodeHave(b[:49]); err == nil {
		t.Error("short have accepted")
	}
	if _, err := programnetwork.DecodeHaveReply(ack[:37]); err == nil {
		t.Error("short have reply accepted")
	}
	if err := programnetwork.DecodeGetPeers(append(g, 0)); err == nil {
		t.Error("long get peers accepted")
	}
	t.Logf("have 50 bytes, have reply 38, subscribe and its reply 10, get peers 6")
}

// entries returns n public entries, IPv6 when v6, seen at the fixed time.
func entries(n int, v6 bool) []programnetwork.PeerEntry {
	var out []programnetwork.PeerEntry
	for i := range n {
		a := netip.AddrFrom4([4]byte{203, 0, byte(113 + i/250), byte(1 + i%250)})
		if v6 {
			a = netip.AddrFrom16([16]byte{0x20, 0x01, 0x0d, 0xb8, 14: byte(i / 256), 15: byte(i)})
		}
		out = append(out, programnetwork.PeerEntry{Address: netip.AddrPortFrom(a, 7010), LastSeen: seen, Flags: programnetwork.PeerShareable})
	}
	return out
}

var seen = time.Unix(1795478400, 0).UTC()

func TestPeerEntries(t *testing.T) {
	for _, c := range []struct {
		v6     bool
		length int
	}{{false, 13}, {true, 25}} {
		e := entries(1, c.v6)[0]
		b := programnetwork.EncodePeerEntry(e)
		if len(b) != c.length {
			t.Fatalf("%v: %d bytes, want %d", e.Address, len(b), c.length)
		}
		got, n, err := programnetwork.DecodePeerEntry(b)
		if err != nil || n != c.length || got != e {
			t.Fatalf("%v: round trip %+v, %d, %v", e.Address, got, n, err)
		}
	}
	b := programnetwork.EncodePeerEntry(entries(1, false)[0])
	b[5] = 4 // TORv3, a network id this prototype does not carry
	if _, _, err := programnetwork.DecodePeerEntry(b); err == nil {
		t.Error("an entry of network 4 was decoded")
	}
}

func TestGetPeersReplyFitsOnePacket(t *testing.T) {
	all := entries(40, true)
	for _, c := range []struct{ size, want int }{
		{programblocks.DefaultPacketSize, programnetwork.MaxPeersPerReply},
		{programblocks.SmallPacketSize, programnetwork.MaxPeersPerReply},
		{512, (512 - 7) / 25},
		{32, 1},
		{31, 0},
	} {
		b, n := programnetwork.EncodeGetPeersReply(1, all, c.size)
		if len(b) > c.size || n != c.want {
			t.Fatalf("packet %d: reply of %d bytes with %d IPv6 entries, want %d", c.size, len(b), n, c.want)
		}
		got, err := programnetwork.DecodeGetPeersReply(b)
		if err != nil || len(got) != n || (n > 0 && !reflect.DeepEqual(got, all[:n])) {
			t.Fatalf("packet %d: round trip %v, %d entries", c.size, err, len(got))
		}
		t.Logf("packet %d: %d IPv6 entries in %d bytes", c.size, n, len(b))
	}
}

// recordingConn records the longest datagram a node sends.
type recordingConn struct {
	net.PacketConn
	longest atomic.Int64
}

func (c *recordingConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	for {
		old := c.longest.Load()
		if int64(len(b)) <= old || c.longest.CompareAndSwap(old, int64(len(b))) {
			break
		}
	}
	return c.PacketConn.WriteTo(b, addr)
}

func TestGetPeersOverANode(t *testing.T) {
	list := &programnetwork.PeerList{}
	for _, e := range entries(60, true) {
		if err := list.SetPeer(e); err != nil {
			t.Fatal(err)
		}
	}
	now := func() time.Time { return seen.Add(time.Hour) }
	for _, size := range []int{programblocks.DefaultPacketSize, programblocks.SmallPacketSize, 128} {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		rec := &recordingConn{PacketConn: pc}
		server := newNode(t, rec)
		server.SetPeers(list)
		server.PacketSize, server.Now = size, now
		go server.Run()
		client := newNode(t, nil)
		client.Now = now
		go client.Run()
		got, err := client.GetPeers(pc.LocalAddr().String())
		if err != nil {
			t.Fatal(err)
		}
		want := min(programnetwork.MaxPeersPerReply, (size-7)/25)
		if len(got) != want || int(rec.longest.Load()) > size || len(client.Peers().Peers) != want {
			t.Fatalf("packet %d: %d peers, %d learned, longest reply %d bytes", size, len(got), len(client.Peers().Peers), rec.longest.Load())
		}
		t.Logf("packet %d: %d of 60 peers in one reply of %d bytes", size, len(got), rec.longest.Load())
		server.Close()
		client.Close()
	}
}

func newNode(t *testing.T, conn net.PacketConn) *programnetwork.Node {
	t.Helper()
	if conn == nil {
		var err error
		if conn, err = net.ListenPacket("udp", "127.0.0.1:0"); err != nil {
			t.Fatal(err)
		}
	}
	st, err := programstore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n, err := programnetwork.NewNode(conn, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
