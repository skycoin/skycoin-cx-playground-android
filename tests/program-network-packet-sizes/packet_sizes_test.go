// Package programnetworkpacketsizes checks the have, subscribe and get
// peers packets: each decodes to what was encoded, has the length the
// document gives, and no get peers reply exceeds 1200 or 1024 bytes, also
// with the longest entries and over a real node with many peers.
package programnetworkpacketsizes

import (
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

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
	g := programnetwork.EncodeGetPeers(9, 12)
	if first, err := programnetwork.DecodeGetPeers(g); len(g) != 8 || err != nil || first != 12 {
		t.Fatalf("get peers %x: first %d, %v", g, first, err)
	}
	if _, err := programnetwork.DecodeHave(b[:49]); err == nil {
		t.Error("short have accepted")
	}
	if _, err := programnetwork.DecodeHaveReply(ack[:37]); err == nil {
		t.Error("short have reply accepted")
	}
	if _, err := programnetwork.DecodeGetPeers(g[:7]); err == nil {
		t.Error("short get peers accepted")
	}
	t.Logf("have 50 bytes, have reply 38, subscribe and its reply 10, get peers 8")
}

func longEntries(n int) []programnetwork.PeerEntry {
	var out []programnetwork.PeerEntry
	for i := range n {
		host := fmt.Sprintf("host%03d.", i) + strings.Repeat("a", programnetwork.MaxAddressLength-13)
		out = append(out, programnetwork.PeerEntry{Scope: programnetwork.ScopeGlobal, Address: host + ":7010",
			Group: strings.Repeat("g", programnetwork.MaxGroupNameLength)})
	}
	return out
}

func TestGetPeersReplyFitsOnePacket(t *testing.T) {
	entries := longEntries(40)
	if len(entries[0].Address) != programnetwork.MaxAddressLength {
		t.Fatalf("test address of %d bytes", len(entries[0].Address))
	}
	for _, size := range []int{programblocks.DefaultPacketSize, programblocks.SmallPacketSize} {
		b, n, err := programnetwork.EncodeGetPeersReply(1, 40, 0, entries, size)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > size || n != (size-11)/programnetwork.MaxPeerEntryLength {
			t.Fatalf("packet %d: reply of %d bytes with %d longest entries", size, len(b), n)
		}
		r, err := programnetwork.DecodeGetPeersReply(b)
		if err != nil || r.Total != 40 || !reflect.DeepEqual(r.Entries, entries[:n]) {
			t.Fatalf("packet %d: round trip %v, %d entries", size, err, len(r.Entries))
		}
		t.Logf("packet %d: %d longest entries in %d bytes", size, n, len(b))
	}
	if _, _, err := programnetwork.EncodeGetPeersReply(1, 1, 0, []programnetwork.PeerEntry{{Address: strings.Repeat("a", 65)}}, 1200); err == nil {
		t.Error("an address of 65 bytes was encoded")
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

func TestGetPeersPagesOverANode(t *testing.T) {
	list := &programnetwork.PeerList{}
	if err := list.SetGroup(programnetwork.Group{Name: "global", Scope: programnetwork.ScopeGlobal, Owner: "-", Operator: "-", SecurityGroup: "global"}); err != nil {
		t.Fatal(err)
	}
	for _, e := range longEntries(60) {
		if err := list.SetPeer(programnetwork.Peer{Address: e.Address, Group: "global", Class: programnetwork.ClassPublic}); err != nil {
			t.Fatal(err)
		}
	}
	for _, size := range []int{programblocks.DefaultPacketSize, programblocks.SmallPacketSize} {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		rec := &recordingConn{PacketConn: pc}
		server := newNode(t, rec)
		server.SetPeers(list)
		server.PacketSize = size
		go server.Run()
		client := newNode(t, nil)
		go client.Run()
		got, err := client.GetPeers(pc.LocalAddr().String())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 60 || int(rec.longest.Load()) > size {
			t.Fatalf("packet %d: %d peers, longest reply %d bytes", size, len(got), rec.longest.Load())
		}
		t.Logf("packet %d: 60 peers in pages, longest reply %d bytes", size, rec.longest.Load())
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
