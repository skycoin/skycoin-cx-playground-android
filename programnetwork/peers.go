package programnetwork

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"sort"
	"strings"
)

// AddressScope says where an address is reachable.
type AddressScope byte

// The address scopes.
const (
	ScopeLAN      AddressScope = 1 // one local network, the group's own
	ScopeInternal AddressScope = 2 // a private network of one security group
	ScopeGlobal   AddressScope = 3 // the internet
)

var scopeNames = map[AddressScope]string{ScopeLAN: "lan", ScopeInternal: "internal", ScopeGlobal: "global"}

func (s AddressScope) String() string {
	if n, ok := scopeNames[s]; ok {
		return n
	}
	return fmt.Sprintf("scope%d", byte(s))
}

// ParseScope parses lan, internal or global.
func ParseScope(s string) (AddressScope, error) {
	for v, n := range scopeNames {
		if n == s {
			return v, nil
		}
	}
	return 0, fmt.Errorf("unknown scope %q, want lan, internal or global", s)
}

// PeerClass is the sharing class of a peer record.
type PeerClass byte

// The five sharing classes, in the order of the issue.
const (
	ClassPrivate           PeerClass = 1 // never shared
	ClassOwnInfrastructure PeerClass = 2 // shared only with the owner's own nodes
	ClassLANGroup          PeerClass = 3 // shared only within the peer's group
	ClassUserShareable     PeerClass = 4 // shared with every peer in the list (D31)
	ClassPublic            PeerClass = 5 // shared with anyone
)

var classNames = map[PeerClass]string{
	ClassPrivate: "private", ClassOwnInfrastructure: "own-infrastructure", ClassLANGroup: "lan-group",
	ClassUserShareable: "user-shareable", ClassPublic: "public",
}

func (c PeerClass) String() string {
	if n, ok := classNames[c]; ok {
		return n
	}
	return fmt.Sprintf("class%d", byte(c))
}

// ParseClass parses a class name.
func ParseClass(s string) (PeerClass, error) {
	for v, n := range classNames {
		if n == s {
			return v, nil
		}
	}
	return 0, fmt.Errorf("unknown class %q, want private, own-infrastructure, lan-group, user-shareable or public", s)
}

// Group is a named set of addresses with one scope, such as lan1, china1,
// china-internal or global.
type Group struct {
	Name          string
	Scope         AddressScope
	Owner         string
	Operator      string
	SecurityGroup string
}

// Peer is one record of the peer list.
type Peer struct {
	Address string // host:port of the peer's node
	Group   string
	Class   PeerClass
	Comment string
}

// PeerList is the peer list of a node, kept in a text file, see
// "Peer list file" in docs/ProgramDistribution.md.
type PeerList struct {
	Groups []Group
	Peers  []Peer
}

// StrangerGroup is the group of an asker that is not in the peer list.
var StrangerGroup = Group{Name: "global", Scope: ScopeGlobal, SecurityGroup: "global"}

func validName(s string, max int) bool {
	return s != "" && len(s) <= max && !strings.ContainsAny(s, " \t\r\n#")
}

// Group returns the group called name.
func (l *PeerList) Group(name string) (Group, bool) {
	for _, g := range l.Groups {
		if g.Name == name {
			return g, true
		}
	}
	return Group{}, false
}

// SetGroup adds a group or replaces the one of the same name.
func (l *PeerList) SetGroup(g Group) error {
	if !validName(g.Name, MaxGroupNameLength) {
		return fmt.Errorf("group name %q: want 1 to %d characters without spaces or #", g.Name, MaxGroupNameLength)
	}
	if _, ok := scopeNames[g.Scope]; !ok {
		return fmt.Errorf("group %s: unknown scope %d", g.Name, g.Scope)
	}
	for _, f := range []string{g.Owner, g.Operator, g.SecurityGroup} {
		if !validName(f, 64) {
			return fmt.Errorf("group %s: owner, operator and security group must be single words, - for none", g.Name)
		}
	}
	for i := range l.Groups {
		if l.Groups[i].Name == g.Name {
			l.Groups[i] = g
			return nil
		}
	}
	l.Groups = append(l.Groups, g)
	return nil
}

// RemoveGroup removes a group that no peer uses.
func (l *PeerList) RemoveGroup(name string) error {
	for _, p := range l.Peers {
		if p.Group == name {
			return fmt.Errorf("group %s still holds peer %s", name, p.Address)
		}
	}
	for i := range l.Groups {
		if l.Groups[i].Name == name {
			l.Groups = append(l.Groups[:i], l.Groups[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no group %s", name)
}

// SetPeer adds a peer or replaces the one with the same address.
func (l *PeerList) SetPeer(p Peer) error {
	if !validName(p.Address, MaxAddressLength) {
		return fmt.Errorf("peer address %q: want host:port of at most %d characters", p.Address, MaxAddressLength)
	}
	if _, _, err := net.SplitHostPort(p.Address); err != nil {
		return fmt.Errorf("peer address %q: %v", p.Address, err)
	}
	if _, ok := l.Group(p.Group); !ok {
		return fmt.Errorf("peer %s: no group %q, add it first", p.Address, p.Group)
	}
	if _, ok := classNames[p.Class]; !ok {
		return fmt.Errorf("peer %s: unknown class %d", p.Address, p.Class)
	}
	if strings.ContainsAny(p.Comment, "\r\n") {
		return fmt.Errorf("peer %s: the comment must be one line", p.Address)
	}
	for i := range l.Peers {
		if l.Peers[i].Address == p.Address {
			l.Peers[i] = p
			return nil
		}
	}
	l.Peers = append(l.Peers, p)
	return nil
}

// RemovePeer removes the peer with address.
func (l *PeerList) RemovePeer(address string) error {
	for i := range l.Peers {
		if l.Peers[i].Address == address {
			l.Peers = append(l.Peers[:i], l.Peers[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("no peer %s", address)
}

// Peer returns the record of the peer with address.
func (l *PeerList) Peer(address string) (Peer, bool) {
	for _, p := range l.Peers {
		if p.Address == address {
			return p, true
		}
	}
	return Peer{}, false
}

// Asker returns the record that matches the address a packet came from:
// the record with that address, or else the only record on the same host.
// An asker not in the list has no record.
func (l *PeerList) Asker(from string) (Peer, bool) {
	if p, ok := l.Peer(from); ok {
		return p, true
	}
	host, _, err := net.SplitHostPort(from)
	if err != nil {
		return Peer{}, false
	}
	var found []Peer
	for _, p := range l.Peers {
		if h, _, _ := net.SplitHostPort(p.Address); h == host {
			found = append(found, p)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return Peer{}, false
}

// SharedWith returns the peers this node may give to the node at from, in
// list order: the asker's own record is left out, and a peer is shared only
// when both its class and its group's scope allow it, as described in
// "Sharing rules" in docs/ProgramDistribution.md.
func (l *PeerList) SharedWith(from string) []PeerEntry {
	asker, known := l.Asker(from)
	askerGroup := StrangerGroup
	if known {
		if g, ok := l.Group(asker.Group); ok {
			askerGroup = g
		}
	}
	var out []PeerEntry
	for _, p := range l.Peers {
		if known && p.Address == asker.Address {
			continue
		}
		g, ok := l.Group(p.Group)
		if !ok || !classAllows(p, asker, known) || !scopeAllows(g, askerGroup) {
			continue
		}
		out = append(out, PeerEntry{Scope: g.Scope, Address: p.Address, Group: g.Name})
	}
	return out
}

func classAllows(p, asker Peer, known bool) bool {
	switch p.Class {
	case ClassOwnInfrastructure:
		return known && asker.Class == ClassOwnInfrastructure
	case ClassLANGroup:
		return known && asker.Group == p.Group
	case ClassUserShareable:
		return known
	case ClassPublic:
		return true
	}
	return false
}

// scopeAllows reports whether an asker in group asker can reach an address
// of group g: a LAN address only from the same group, an internal address
// only from the same security group (D33), a global address from anywhere.
func scopeAllows(g, asker Group) bool {
	switch g.Scope {
	case ScopeLAN:
		return asker.Name == g.Name
	case ScopeInternal:
		return asker.SecurityGroup == g.SecurityGroup
	case ScopeGlobal:
		return true
	}
	return false
}

// Format returns the text of the peer list file: the groups, then the
// peers, each sorted by name.
func (l *PeerList) Format() []byte {
	var b strings.Builder
	b.WriteString("# group NAME SCOPE OWNER OPERATOR SECURITY-GROUP\n")
	groups := append([]Group(nil), l.Groups...)
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	for _, g := range groups {
		fmt.Fprintf(&b, "group %s %s %s %s %s\n", g.Name, g.Scope, g.Owner, g.Operator, g.SecurityGroup)
	}
	b.WriteString("# peer ADDRESS GROUP CLASS [COMMENT]\n")
	peers := append([]Peer(nil), l.Peers...)
	sort.SliceStable(peers, func(i, j int) bool { return peers[i].Address < peers[j].Address })
	for _, p := range peers {
		line := fmt.Sprintf("peer %s %s %s", p.Address, p.Group, p.Class)
		if p.Comment != "" {
			line += " " + p.Comment
		}
		b.WriteString(line + "\n")
	}
	return []byte(b.String())
}

// ParsePeerList parses the text of a peer list file.
func ParsePeerList(text string) (*PeerList, error) {
	l := &PeerList{}
	var peers []Peer
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		switch {
		case f[0] == "group" && len(f) == 6:
			scope, err := ParseScope(f[2])
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n+1, err)
			}
			if err := l.SetGroup(Group{Name: f[1], Scope: scope, Owner: f[3], Operator: f[4], SecurityGroup: f[5]}); err != nil {
				return nil, fmt.Errorf("line %d: %v", n+1, err)
			}
		case f[0] == "peer" && len(f) >= 4:
			class, err := ParseClass(f[3])
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", n+1, err)
			}
			comment := ""
			if len(f) > 4 {
				comment = strings.Join(f[4:], " ")
			}
			peers = append(peers, Peer{Address: f[1], Group: f[2], Class: class, Comment: comment})
		default:
			return nil, fmt.Errorf("line %d: want group NAME SCOPE OWNER OPERATOR SECURITY-GROUP or peer ADDRESS GROUP CLASS [COMMENT]", n+1)
		}
	}
	// Peers are checked after every group is read, so the order of lines does not matter.
	for _, p := range peers {
		if err := l.SetPeer(p); err != nil {
			return nil, err
		}
	}
	return l, nil
}

// ReadPeerList reads a peer list file. A missing file is an empty list.
func ReadPeerList(path string) (*PeerList, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &PeerList{}, nil
	}
	if err != nil {
		return nil, err
	}
	l, err := ParsePeerList(string(b))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

// WritePeerList writes the list to path through a temporary file.
func WritePeerList(path string, l *PeerList) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, l.Format(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
