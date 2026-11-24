// Package programtransportpacketsizes checks that no reply exceeds the
// packet size, 1200 or 1024 bytes, for every operation including requests
// that ask for far more than fits, and records every packet of a whole
// fetch to check the same on the wire.
package programtransportpacketsizes

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// recordingConn records the length of every datagram the server sends.
type recordingConn struct {
	net.PacketConn
	mu      sync.Mutex
	lengths []int
}

func (c *recordingConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.mu.Lock()
	c.lengths = append(c.lengths, len(b))
	c.mu.Unlock()
	return c.PacketConn.WriteTo(b, addr)
}

func catalog(t *testing.T) (*programblocks.Catalog, cipher.PubKey) {
	files, err := programblocks.ReadProgramDirectory("../../apps/benchmarks")
	if err != nil {
		t.Fatal(err)
	}
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("packet sizes"))
	c, err := programblocks.NewCatalog(map[string]map[string][]byte{"benchmarks": files}, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return c, pub
}

func TestEveryReplyFits(t *testing.T) {
	c, _ := catalog(t)
	for _, size := range []int{programblocks.DefaultPacketSize, programblocks.SmallPacketSize} {
		for _, addr := range c.Addresses() {
			for _, r := range []programblocks.Request{
				{Operation: programblocks.OperationListManifests},
				{Operation: programblocks.OperationGetManifest, Address: addr, Offset: 0, Length: 65535},
				{Operation: programblocks.OperationGetManifest, Address: addr, Offset: 5000, Length: 65535},
				{Operation: programblocks.OperationGetBlockList, Address: addr, Offset: 1, Length: 65535},
				{Operation: programblocks.OperationGetBlocks, Address: addr, Offset: 0, Length: 65535},
				{Operation: programblocks.OperationGetBlocks, Address: addr, Offset: 1 << 40, Length: 1},
				{Operation: programblocks.OperationGetBlocks, Address: programblocks.Hash{1}, Offset: 0, Length: 1},
			} {
				reply := c.Reply(programblocks.EncodeRequest(r), size)
				if len(reply) == 0 || len(reply) > size {
					t.Fatalf("packet %d, operation %#x: reply of %d bytes", size, byte(r.Operation), len(reply))
				}
				if req := programblocks.EncodeRequest(r); len(req) > size {
					t.Fatalf("request of %d bytes", len(req))
				}
			}
		}
		if n := programblocks.MaxBlocksPayload(size) / programblocks.DefaultBlockSize; n != map[int]int{1200: 2, 1024: 1}[size] {
			t.Fatalf("packet %d carries %d blocks", size, n)
		}
	}
	// Malformed and wrong-version requests get an error reply that fits.
	for _, b := range [][]byte{{1, 2, 0, 0, 0, 1}, {9, 1, 0, 0, 0, 1}, {1, 0x7e, 0, 0, 0, 1}} {
		reply := c.Reply(b, programblocks.SmallPacketSize)
		r, err := programblocks.DecodeReply(reply)
		if err != nil || r.Operation != programblocks.OperationError || r.RequestID != 1 {
			t.Fatalf("request % x: reply %+v, %v", b, r, err)
		}
	}
}

func TestWirePacketsFit(t *testing.T) {
	c, pub := catalog(t)
	for _, size := range []int{programblocks.DefaultPacketSize, programblocks.SmallPacketSize} {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		rec := &recordingConn{PacketConn: pc}
		go c.Serve(rec, size)
		conn, _ := net.Dial("udp", pc.LocalAddr().String())
		client := programblocks.NewClient(conn, pub)
		client.PacketSize = size
		if _, err := client.FetchProgram("benchmarks"); err != nil {
			t.Fatal(err)
		}
		conn.Close()
		pc.Close()
		rec.mu.Lock()
		largest := 0
		for _, n := range rec.lengths {
			largest = max(largest, n)
		}
		t.Logf("packet %d: %d replies on the wire, largest %d bytes", size, len(rec.lengths), largest)
		if largest > size {
			t.Fatalf("a %d byte reply went out with packet size %d", largest, size)
		}
		rec.mu.Unlock()
	}
}
