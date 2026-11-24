// Package programnetworkfloodinprocess runs N nodes in one process, each
// subscribed to its ring neighbours and to the node three places on,
// publishes programs at one node and checks the flood: every node ends up
// holding every program, verified, each node downloads each program
// exactly once, and the flood stops by itself.
package programnetworkfloodinprocess

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// subscriptionOffsets returns the distinct offsets of the nodes each node
// subscribes to: its two ring neighbours and the node three places on.
func subscriptionOffsets(count int) []int {
	seen := map[int]bool{}
	var out []int
	for _, d := range []int{1, count - 1, 3} {
		if d %= count; d != 0 && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// startNodes starts count nodes that trust trusted. Node 0 also trusts
// publisherOnly, so it can publish programs the others refuse.
func startNodes(t *testing.T, count int, trusted cipher.PubKey, publisherOnly ...cipher.PubKey) []*programnetwork.Node {
	t.Helper()
	nodes := make([]*programnetwork.Node, count)
	for i := range nodes {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		st, err := programstore.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		keys := []cipher.PubKey{trusted}
		if i == 0 {
			keys = append(keys, publisherOnly...)
		}
		if nodes[i], err = programnetwork.NewNode(pc, st, keys); err != nil {
			t.Fatal(err)
		}
		go nodes[i].Run()
		t.Cleanup(func() { nodes[i].Close() })
	}
	for i, n := range nodes {
		for _, d := range subscriptionOffsets(count) {
			if err := n.Subscribe(nodes[(i+d)%count].Addr().String()); err != nil {
				t.Fatal(err)
			}
		}
	}
	waitSubscribed(t, nodes, len(subscriptionOffsets(count))*count, 5*time.Second)
	return nodes
}

func programs(t *testing.T) []map[string][]byte {
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

func totals(nodes []*programnetwork.Node) programnetwork.Stats {
	var s programnetwork.Stats
	for _, n := range nodes {
		x := n.Stats()
		s.HavesReceived += x.HavesReceived
		s.HavesDuplicate += x.HavesDuplicate
		s.HavesSent += x.HavesSent
		s.HavesUnacknowledged += x.HavesUnacknowledged
		s.Downloads += x.Downloads
		s.DownloadFailures += x.DownloadFailures
	}
	return s
}

// waitFlood waits until every node holds every address and the counters
// of all nodes stay unchanged for quiet, which shows the flood ended.
func waitFlood(t *testing.T, nodes []*programnetwork.Node, addrs []programblocks.Hash, quiet, limit time.Duration) time.Duration {
	t.Helper()
	start := time.Now()
	var held time.Duration
	last, lastChange := totals(nodes), time.Now()
	for time.Since(start) < limit {
		time.Sleep(20 * time.Millisecond)
		all := true
		for _, n := range nodes {
			for _, a := range addrs {
				all = all && n.Holds(a)
			}
		}
		if all && held == 0 {
			held = time.Since(start)
		}
		if s := totals(nodes); s != last {
			last, lastChange = s, time.Now()
		}
		if held > 0 && time.Since(lastChange) >= quiet {
			return held
		}
	}
	t.Fatalf("flood did not finish in %v: %+v", limit, totals(nodes))
	return 0
}

// checkFlood publishes the programs at node 0 and checks the result.
func checkFlood(t *testing.T, nodes []*programnetwork.Node, secret cipher.SecKey, limit time.Duration) {
	var addrs []programblocks.Hash
	progs := programs(t)
	for _, files := range progs {
		addr, err := nodes[0].Publish(files, secret, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		addrs = append(addrs, addr)
	}
	held := waitFlood(t, nodes, addrs, time.Second, limit)
	for i, n := range nodes[1:] {
		if s := n.Stats(); s.Downloads != len(addrs) {
			t.Errorf("node %d downloaded %d times for %d programs: %+v", i+1, s.Downloads, len(addrs), s)
		}
	}
	if s := nodes[0].Stats(); s.Downloads != 0 {
		t.Errorf("the publisher downloaded %d programs", s.Downloads)
	}
	s := totals(nodes)
	edges := len(subscriptionOffsets(len(nodes))) * len(nodes)
	if s.HavesReceived != edges*len(addrs) || s.HavesUnacknowledged != 0 || s.DownloadFailures != 0 {
		t.Errorf("%d haves for %d subscriptions and %d programs, want one per subscription and program", s.HavesReceived, edges, len(addrs))
	}
	t.Logf("%d nodes, %d programs: all held after %d ms, %d downloads, %d haves received (%d duplicate), %d resent haves given up, %d failed downloads",
		len(nodes), len(addrs), held.Milliseconds(), s.Downloads, s.HavesReceived, s.HavesDuplicate, s.HavesUnacknowledged, s.DownloadFailures)
	// Every node's store holds the same verified files as the publisher's.
	for i, files := range progs {
		for j, n := range nodes {
			got, err := n.Store().Files(addrs[i])
			if err != nil {
				t.Fatalf("node %d program %d: %v", j, i, err)
			}
			for path, b := range files {
				if !bytes.Equal(got[path], b) {
					t.Fatalf("node %d program %d: %s differs", j, i, path)
				}
			}
		}
	}
}

func TestFloodAcrossNodes(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program network flood"))
	for _, count := range []int{3, 8, 16} {
		t.Run(fmt.Sprintf("%d-nodes", count), func(t *testing.T) {
			nodes := startNodes(t, count, pub)
			checkFlood(t, nodes, secret, 20*time.Second)
		})
	}
}

func TestUntrustedProgramIsNotForwarded(t *testing.T) {
	pub, _ := cipher.MustGenerateDeterministicKeyPair([]byte("program network flood"))
	roguePub, rogueSecret := cipher.MustGenerateDeterministicKeyPair([]byte("not a trusted signer"))
	nodes := startNodes(t, 4, pub, roguePub)
	addr, err := nodes[0].Publish(programs(t)[1], rogueSecret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	received := 0
	for i, n := range nodes[1:] {
		s := n.Stats()
		received += s.HavesReceived
		if n.Holds(addr) || s.Downloads != 0 || s.HavesSent != 0 {
			t.Errorf("node %d took or forwarded the untrusted program: %+v", i+1, s)
		}
	}
	if received == 0 {
		t.Fatal("no node heard the have")
	}
	t.Logf("untrusted program: %d haves heard, 0 downloads, 0 forwarded", received)
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
