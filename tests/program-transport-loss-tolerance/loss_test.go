// Package programtransportlosstolerance drops, repeats and corrupts packets
// between the server and the client and checks that the fetch still
// delivers every file unchanged, through retries.
package programtransportlosstolerance

import (
	"bytes"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// faultyConn wraps the server's socket. It drops a share of the requests
// it reads and of the replies it sends, sends some replies twice, and can
// flip a byte in the first reply carrying blocks.
type faultyConn struct {
	net.PacketConn
	mu          sync.Mutex
	rand        *rand.Rand
	drop        float64
	corruptOnce bool
	dropped     int
}

func (c *faultyConn) lose() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rand.Float64() < c.drop {
		c.dropped++
		return true
	}
	return false
}

func (c *faultyConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(b)
		if err != nil || !c.lose() {
			return n, addr, err
		}
	}
}

func (c *faultyConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if c.lose() {
		return len(b), nil
	}
	c.mu.Lock()
	if c.corruptOnce && b[1] == byte(programblocks.OperationGetBlocks|programblocks.ReplyFlag) && len(b) > 100 {
		c.corruptOnce = false
		b = append([]byte(nil), b...)
		b[len(b)-1] ^= 0x40
	}
	twice := c.rand.Float64() < 0.1
	c.mu.Unlock()
	if twice {
		c.PacketConn.WriteTo(b, addr)
	}
	return c.PacketConn.WriteTo(b, addr)
}

func TestFetchSurvivesLoss(t *testing.T) {
	files, err := programblocks.ReadProgramDirectory("../../apps/benchmarks")
	if err != nil {
		t.Fatal(err)
	}
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("loss tolerance"))
	catalog, err := programblocks.NewCatalog(map[string]map[string][]byte{"benchmarks": files}, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, drop := range []float64{0.2, 0.4} {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		fc := &faultyConn{PacketConn: pc, rand: rand.New(rand.NewSource(7)), drop: drop, corruptOnce: true}
		go catalog.Serve(fc, programblocks.DefaultPacketSize)
		conn, _ := net.Dial("udp", pc.LocalAddr().String())
		c := programblocks.NewClient(conn, pub)
		c.FirstTimeout = 30 * time.Millisecond
		c.MaxTimeout = 200 * time.Millisecond
		c.MaxTries = 12
		start := time.Now()
		p, err := c.FetchProgram("benchmarks")
		if err != nil {
			t.Fatalf("drop %.0f%%: %v", drop*100, err)
		}
		for path, b := range files {
			if !bytes.Equal(p.Files[path], b) {
				t.Fatalf("drop %.0f%%: %s differs", drop*100, path)
			}
		}
		if c.Stats.RangesRejected != 1 {
			t.Fatalf("drop %.0f%%: %d corrupted ranges rejected, want 1", drop*100, c.Stats.RangesRejected)
		}
		if c.Stats.Retransmissions == 0 || fc.dropped == 0 {
			t.Fatalf("drop %.0f%%: no loss happened, the test proves nothing", drop*100)
		}
		t.Logf("drop %.0f%%: %d packets dropped, %d requests, %d resent, %d replies discarded, %v",
			drop*100, fc.dropped, c.Stats.RequestsSent, c.Stats.Retransmissions, c.Stats.RepliesDiscarded, time.Since(start).Round(time.Millisecond))
		conn.Close()
		pc.Close()
	}
}
