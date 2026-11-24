// Package programnetworkannouncerequestinprocess runs N nodes in one
// process, each subscribed to its ring neighbours and to the node three
// places on, publishes programs at one node and checks announce and request
// (#42): every have goes in the have queue and is read, no have starts a
// download by itself, each super node requests each program exactly once
// after reading its have, every node ends up holding every program,
// verified, and the relaying stops by itself (D55).
package programnetworkannouncerequestinprocess

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
		go nodes[i].RunUpdates(nil)
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
		s.HavesRead += x.HavesRead
		s.Requests += x.Requests
		s.HavesDuplicate += x.HavesDuplicate
		s.HavesSent += x.HavesSent
		s.HavesUnacknowledged += x.HavesUnacknowledged
		s.Downloads += x.Downloads
		s.DownloadFailures += x.DownloadFailures
	}
	return s
}

// waitHeld waits until every node holds every address and the counters
// of all nodes stay unchanged for quiet, which shows the relaying ended.
func waitHeld(t *testing.T, nodes []*programnetwork.Node, addrs []programblocks.Hash, quiet, limit time.Duration) time.Duration {
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
	t.Fatalf("the programs did not spread in %v: %+v", limit, totals(nodes))
	return 0
}

// checkSpread publishes the programs at node 0 and checks the result.
func checkSpread(t *testing.T, nodes []*programnetwork.Node, secret cipher.SecKey, limit time.Duration) {
	var addrs []programblocks.Hash
	progs := programs(t)
	for _, files := range progs {
		addr, err := nodes[0].Publish(files, secret, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		addrs = append(addrs, addr)
	}
	held := waitHeld(t, nodes, addrs, time.Second, limit)
	for i, n := range nodes[1:] {
		if s := n.Stats(); s.Downloads != len(addrs) || s.Requests != len(addrs) {
			t.Errorf("node %d requested %d and downloaded %d times for %d programs: %+v", i+1, s.Requests, s.Downloads, len(addrs), s)
		}
	}
	if s := nodes[0].Stats(); s.Downloads != 0 {
		t.Errorf("the publisher downloaded %d programs", s.Downloads)
	}
	s := totals(nodes)
	edges := len(subscriptionOffsets(len(nodes))) * len(nodes)
	if s.HavesReceived != edges*len(addrs) || s.HavesRead != s.HavesReceived || s.HavesUnacknowledged != 0 || s.DownloadFailures != 0 {
		t.Errorf("%d haves received and %d read for %d subscriptions and %d programs, want one per subscription and program, all read", s.HavesReceived, s.HavesRead, edges, len(addrs))
	}
	t.Logf("%d nodes, %d programs: all held after %d ms, %d requests, %d downloads, %d haves received and read (%d duplicate), %d resent haves given up, %d failed downloads",
		len(nodes), len(addrs), held.Milliseconds(), s.Requests, s.Downloads, s.HavesReceived, s.HavesDuplicate, s.HavesUnacknowledged, s.DownloadFailures)
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

func TestAnnounceAndRequestAcrossNodes(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program network announce"))
	for _, count := range []int{3, 8, 16} {
		t.Run(fmt.Sprintf("%d-nodes", count), func(t *testing.T) {
			nodes := startNodes(t, count, pub)
			checkSpread(t, nodes, secret, 20*time.Second)
		})
	}
}

func TestUntrustedProgramIsNotForwarded(t *testing.T) {
	pub, _ := cipher.MustGenerateDeterministicKeyPair([]byte("program network announce"))
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

func startNode(t *testing.T, trusted cipher.PubKey, settings programnetwork.Settings) *programnetwork.Node {
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
	go n.Run()
	t.Cleanup(func() { n.Close() })
	return n
}

// TestHaveNeverStartsADownload checks that a have only goes in the have
// queue: nothing is requested until the program reads the queue, and a
// medium node that reads a have it does not keep requests nothing.
func TestHaveNeverStartsADownload(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program network announce"))
	publisher := startNode(t, pub, programnetwork.Settings{Type: programnetwork.TypeSuper})
	super := startNode(t, pub, programnetwork.Settings{Type: programnetwork.TypeSuper})
	medium := startNode(t, pub, programnetwork.Settings{Type: programnetwork.TypeMedium})
	for _, n := range []*programnetwork.Node{super, medium} {
		if err := n.Subscribe(publisher.Addr().String()); err != nil {
			t.Fatal(err)
		}
	}
	waitSubscribed(t, []*programnetwork.Node{publisher}, 2, 5*time.Second)
	addr, err := publisher.Publish(programs(t)[1], secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); super.Haves().Pending() == 0 || medium.Haves().Pending() == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("haves queued: super %d, medium %d", super.Haves().Pending(), medium.Haves().Pending())
		}
	}
	time.Sleep(300 * time.Millisecond)
	if s := super.Stats(); s.Requests != 0 || s.Downloads != 0 || super.Holds(addr) {
		t.Fatalf("a have started a download before the queue was read: %+v", s)
	}
	if count := super.Update(0); count != 1 {
		t.Fatalf("super update has %d haves, want 1", count)
	}
	read, _ := super.Haves().ReadyNext()
	if read.Address != addr || read.From != publisher.Addr().String() {
		t.Fatalf("super read %+v, want the have of %s from %s", read, addr, publisher.Addr())
	}
	for deadline := time.Now().Add(5 * time.Second); !super.Holds(addr); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("super did not hold the program after reading its have: %+v", super.Stats())
		}
	}
	if s := super.Stats(); s.HavesRead != 1 || s.Requests != 1 || s.Downloads != 1 {
		t.Errorf("super: %+v, want one have read, one request, one download", s)
	}
	if got := medium.Update(0); got != 1 {
		t.Fatalf("medium read %d haves, want 1", got)
	}
	time.Sleep(300 * time.Millisecond)
	if s := medium.Stats(); s.HavesRead != 1 || s.Requests != 0 || s.Downloads != 0 || medium.Holds(addr) {
		t.Errorf("medium without pins or budget: %+v, want the have read and nothing requested", s)
	}
}
