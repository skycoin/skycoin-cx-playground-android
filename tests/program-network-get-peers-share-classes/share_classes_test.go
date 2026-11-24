// Package programnetworkgetpeersshareclasses asks one node for its peers
// from askers of every kind, over UDP, and checks that each gets exactly
// the peers its sharing classes allow: a private peer never, an own
// infrastructure peer only to the owner's own nodes, a LAN group peer only
// within its group, a user-shareable peer to known peers and a public peer
// to anyone.
package programnetworkgetpeersshareclasses

import (
	"net"
	"reflect"
	"sort"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

func listen(t *testing.T) (*programnetwork.Node, string) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	st, err := programstore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n, err := programnetwork.NewNode(pc, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	go n.Run()
	t.Cleanup(func() { n.Close() })
	return n, pc.LocalAddr().String()
}

func TestGetPeersFollowsShareClasses(t *testing.T) {
	server, serverAddr := listen(t)
	askers := map[string]*programnetwork.Node{}
	addrs := map[string]string{}
	for _, name := range []string{"own", "lan", "user", "private", "stranger"} {
		askers[name], addrs[name] = listen(t)
	}
	text := `group global global - - global
group home lan moe moe home
peer 10.0.0.1:7010 global private never shared
peer 10.0.0.2:7010 global own-infrastructure
peer 192.168.1.9:7010 home lan-group
peer 10.0.0.4:7010 global user-shareable
peer 10.0.0.5:7010 global public
peer ` + addrs["own"] + ` global own-infrastructure
peer ` + addrs["lan"] + ` home lan-group
peer ` + addrs["user"] + ` global user-shareable
peer ` + addrs["private"] + ` global private
`
	list, err := programnetwork.ParsePeerList(text)
	if err != nil {
		t.Fatal(err)
	}
	server.SetPeers(list)
	want := map[string][]string{
		"own":      {"10.0.0.2:7010", "10.0.0.4:7010", addrs["user"], "10.0.0.5:7010"},
		"lan":      {"192.168.1.9:7010", "10.0.0.4:7010", addrs["user"], "10.0.0.5:7010"},
		"user":     {"10.0.0.4:7010", "10.0.0.5:7010"},
		"private":  {"10.0.0.4:7010", addrs["user"], "10.0.0.5:7010"},
		"stranger": {"10.0.0.5:7010"},
	}
	for name, asker := range askers {
		entries, err := asker.GetPeers(serverAddr)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, e := range entries {
			got = append(got, e.Address)
			if p, _ := list.Peer(e.Address); p.Class == programnetwork.ClassPrivate {
				t.Errorf("%s was given the private peer %s", name, e.Address)
			}
		}
		sort.Strings(got)
		sort.Strings(want[name])
		if !reflect.DeepEqual(got, want[name]) {
			t.Errorf("%s asker got %q, want %q", name, got, want[name])
		}
		t.Logf("%-8s asker gets %d peers: %v", name, len(got), got)
	}
}
