// Package programnetworkgetpeersgroupsscopes asks one node for its peers
// from askers in different groups, over UDP. Every peer is public, so only
// the address scope decides: a LAN address goes only to its own group, an
// internal address only to the same security group, and a global address
// to anyone. A LAN address is never given to a global peer.
package programnetworkgetpeersgroupsscopes

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

const (
	lanAddr          = "192.168.1.5:7010"
	china1Addr       = "10.1.0.1:7010"
	chinaInternalAdr = "10.2.0.1:7010"
	euAddr           = "10.3.0.1:7010"
	globalAddr       = "203.0.113.5:7010"
)

func TestGetPeersFollowsGroupsAndScopes(t *testing.T) {
	server, serverAddr := listen(t)
	groups := []string{"lan1", "china1", "eu-internal", "global"}
	askers := map[string]*programnetwork.Node{}
	addrs := map[string]string{}
	for _, g := range append(groups, "stranger") {
		askers[g], addrs[g] = listen(t)
	}
	list := &programnetwork.PeerList{}
	for _, g := range []programnetwork.Group{
		{Name: "lan1", Scope: programnetwork.ScopeLAN, Owner: "moe", Operator: "moe", SecurityGroup: "home1"},
		{Name: "china1", Scope: programnetwork.ScopeInternal, Owner: "lab", Operator: "isp-a", SecurityGroup: "china"},
		{Name: "china-internal", Scope: programnetwork.ScopeInternal, Owner: "lab", Operator: "isp-b", SecurityGroup: "china"},
		{Name: "eu-internal", Scope: programnetwork.ScopeInternal, Owner: "lab", Operator: "isp-c", SecurityGroup: "eu"},
		{Name: "global", Scope: programnetwork.ScopeGlobal, Owner: "-", Operator: "-", SecurityGroup: "global"},
	} {
		if err := list.SetGroup(g); err != nil {
			t.Fatal(err)
		}
	}
	add := func(addr, group string, class programnetwork.PeerClass) {
		if err := list.SetPeer(programnetwork.Peer{Address: addr, Group: group, Class: class}); err != nil {
			t.Fatal(err)
		}
	}
	add(lanAddr, "lan1", programnetwork.ClassPublic)
	add(china1Addr, "china1", programnetwork.ClassPublic)
	add(chinaInternalAdr, "china-internal", programnetwork.ClassPublic)
	add(euAddr, "eu-internal", programnetwork.ClassPublic)
	add(globalAddr, "global", programnetwork.ClassPublic)
	// The askers are private records, so they place each asker in its
	// group without being shared themselves.
	for _, g := range groups {
		add(addrs[g], g, programnetwork.ClassPrivate)
	}
	server.SetPeers(list)
	want := map[string][]string{
		"lan1":        {lanAddr, globalAddr},
		"china1":      {china1Addr, chinaInternalAdr, globalAddr},
		"eu-internal": {euAddr, globalAddr},
		"global":      {globalAddr},
		"stranger":    {globalAddr},
	}
	for name, asker := range askers {
		entries, err := asker.GetPeers(serverAddr)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, e := range entries {
			got = append(got, e.Address)
			g, _ := list.Group(e.Group)
			if e.Scope != g.Scope {
				t.Errorf("%s has scope %s, its group %s", e.Address, e.Scope, g.Scope)
			}
		}
		sort.Strings(got)
		sort.Strings(want[name])
		if !reflect.DeepEqual(got, want[name]) {
			t.Errorf("asker in %s got %q, want %q", name, got, want[name])
		}
		t.Logf("asker in %-11s gets %v", name, got)
	}
}
