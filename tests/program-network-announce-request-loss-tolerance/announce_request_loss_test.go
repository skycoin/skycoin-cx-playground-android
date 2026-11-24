// Package programnetworkannouncerequestlosstolerance announces programs
// across eight nodes whose sockets drop a share of every packet they read
// and send, and send some packets twice. Subscribe, have and the requested
// downloads all retry, so every node must still hold every program,
// verified, after requesting and downloading each one exactly once.
package programnetworkannouncerequestlosstolerance

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// lossyConn drops a share of the packets read and sent on a node's socket
// and sends a tenth of the rest twice.
type lossyConn struct {
	net.PacketConn
	mu      sync.Mutex
	rand    *rand.Rand
	drop    float64
	dropped int
	passed  int
}

func (c *lossyConn) lose() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.rand.Float64() < c.drop {
		c.dropped++
		return true
	}
	c.passed++
	return false
}

func (c *lossyConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		n, addr, err := c.PacketConn.ReadFrom(b)
		if err != nil || !c.lose() {
			return n, addr, err
		}
	}
}

func (c *lossyConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if c.lose() {
		return len(b), nil
	}
	c.mu.Lock()
	twice := c.rand.Float64() < 0.1
	c.mu.Unlock()
	if twice {
		c.PacketConn.WriteTo(b, addr)
	}
	return c.PacketConn.WriteTo(b, addr)
}

func TestAnnounceAndRequestSurvivesLoss(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program network loss"))
	var progs []map[string][]byte
	for _, app := range []string{"benchmarks", "bench-fib"} {
		files, err := programblocks.ReadProgramDirectory("../../apps/" + app)
		if err != nil {
			t.Fatal(err)
		}
		progs = append(progs, files)
	}
	for _, drop := range []float64{0.2, 0.3} {
		t.Run(fmt.Sprintf("drop-%.0f%%", drop*100), func(t *testing.T) {
			const count = 8
			nodes := make([]*programnetwork.Node, count)
			conns := make([]*lossyConn, count)
			for i := range nodes {
				pc, err := net.ListenPacket("udp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				conns[i] = &lossyConn{PacketConn: pc, rand: rand.New(rand.NewSource(int64(i) + int64(drop*100))), drop: drop}
				st, err := programstore.Open(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				if nodes[i], err = programnetwork.NewNode(conns[i], st, []cipher.PubKey{pub}); err != nil {
					t.Fatal(err)
				}
				go nodes[i].Run()
				go nodes[i].RunUpdates(nil)
				t.Cleanup(func() { nodes[i].Close() })
			}
			for i, n := range nodes {
				for _, d := range []int{1, count - 1, 3} {
					if err := n.Subscribe(nodes[(i+d)%count].Addr().String()); err != nil {
						t.Fatal(err)
					}
				}
			}
			subscribed := waitSubscribed(t, nodes, 3*count, 60*time.Second)
			start := time.Now()
			var addrs []programblocks.Hash
			for _, files := range progs {
				addr, err := nodes[0].Publish(files, secret, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				addrs = append(addrs, addr)
			}
			for {
				all := true
				for _, n := range nodes {
					for _, a := range addrs {
						all = all && n.Holds(a)
					}
				}
				if all {
					break
				}
				if time.Since(start) > 60*time.Second {
					t.Fatal("not every node holds every program after 60 s")
				}
				time.Sleep(20 * time.Millisecond)
			}
			held := time.Since(start)
			var downloads, failures, haves, unacked, dropped, passed int
			for i, n := range nodes {
				s := n.Stats()
				if i > 0 && (s.Downloads != len(addrs) || s.Requests != len(addrs)) {
					t.Errorf("node %d requested %d and downloaded %d times for %d programs", i, s.Requests, s.Downloads, len(addrs))
				}
				for _, a := range addrs {
					if _, err := n.Store().Files(a); err != nil {
						t.Errorf("node %d: %v", i, err)
					}
				}
				downloads += s.Downloads
				failures += s.DownloadFailures
				haves += s.HavesReceived
				unacked += s.HavesUnacknowledged
				conns[i].mu.Lock()
				dropped += conns[i].dropped
				passed += conns[i].passed
				conns[i].mu.Unlock()
			}
			t.Logf("drop %.0f%%: 24 subscriptions in %d ms, %d nodes hold %d programs after %d ms, %d downloads, %d failed, %d haves received, %d haves given up, %d of %d packets dropped",
				drop*100, subscribed.Milliseconds(), count, len(addrs), held.Milliseconds(), downloads, failures, haves, unacked, dropped, dropped+passed)
		})
	}
}

// waitSubscribed waits until the nodes count want subscribers in all.
func waitSubscribed(t *testing.T, nodes []*programnetwork.Node, want int, limit time.Duration) time.Duration {
	t.Helper()
	start := time.Now()
	for {
		got := 0
		for _, n := range nodes {
			got += n.Stats().Subscribers
		}
		if got == want {
			return time.Since(start)
		}
		if time.Since(start) > limit {
			t.Fatalf("%d of %d subscriptions after %v", got, want, limit)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
