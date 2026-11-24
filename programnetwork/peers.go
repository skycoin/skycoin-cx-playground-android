package programnetwork

import (
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The peer exchange limits, fixed as in Bitcoin's addr relay (#41), see
// "Peer exchange" in docs/ProgramDistribution.md.
const (
	// MaxPeersPerReply is the most entries one get peers reply holds.
	MaxPeersPerReply = 32
	// PeerAnswerInterval is how often a node answers one asker with
	// entries. A get peers within it gets a reply with no entries.
	PeerAnswerInterval = 10 * time.Minute
	// MaxPeerTableSize is the most entries a peer list holds.
	MaxPeerTableSize = 1024
	// PeerFreshAge is how long after it was last seen a shareable entry is
	// given out and learned, as Bitcoin's 30-day horizon.
	PeerFreshAge = 30 * 24 * time.Hour
)

// The network ids of BIP155 that a peer entry carries.
const (
	NetworkIPv4 byte = 1
	NetworkIPv6 byte = 2
)

// PeerShareable is bit 0 of a peer entry's flag byte. The other bits stay
// zero.
const PeerShareable byte = 1

// PeerEntry is one peer: a binary address, the time it was last seen and
// one flag byte. Only a shareable entry is ever sent or learned.
type PeerEntry struct {
	Address  netip.AddrPort
	LastSeen time.Time
	Flags    byte
}

// Shareable reports bit 0 of the flag byte.
func (e PeerEntry) Shareable() bool { return e.Flags&PeerShareable != 0 }

// Network is the BIP155 network id of the address.
func (e PeerEntry) Network() byte {
	if e.Address.Addr().Is4() {
		return NetworkIPv4
	}
	return NetworkIPv6
}

// fresh reports whether e was seen within PeerFreshAge of now.
func (e PeerEntry) fresh(now time.Time) bool { return now.Sub(e.LastSeen) <= PeerFreshAge }

// ParsePeerAddress parses an IPv4 or IPv6 address and port.
func ParsePeerAddress(s string) (netip.AddrPort, error) {
	a, err := netip.ParseAddrPort(s)
	if err != nil || a.Port() == 0 {
		return netip.AddrPort{}, fmt.Errorf("peer address %q: want an IPv4 or IPv6 address and port", s)
	}
	return netip.AddrPortFrom(a.Addr().Unmap(), a.Port()), nil
}

// PeerList is the peer list of a node: public peers, which are shareable,
// and private peers, which are configured locally and never sent or
// learned. It is kept in a text file, see "Peer list file" in
// docs/ProgramDistribution.md.
type PeerList struct {
	Peers []PeerEntry // sorted by address
}

func (l *PeerList) index(a netip.AddrPort) (int, bool) {
	return slices.BinarySearchFunc(l.Peers, a, func(e PeerEntry, t netip.AddrPort) int { return e.Address.Compare(t) })
}

// Peer returns the entry of address.
func (l *PeerList) Peer(a netip.AddrPort) (PeerEntry, bool) {
	i, ok := l.index(a)
	if !ok {
		return PeerEntry{}, false
	}
	return l.Peers[i], true
}

// SetPeer adds an entry or replaces the one with the same address. A list
// at MaxPeerTableSize takes no new address.
func (l *PeerList) SetPeer(e PeerEntry) error {
	if !e.Address.IsValid() || e.Address.Port() == 0 {
		return fmt.Errorf("peer address %v: want an IPv4 or IPv6 address and port", e.Address)
	}
	if e.Flags&^PeerShareable != 0 {
		return fmt.Errorf("peer %v: flag byte %#x, only bit 0 is defined", e.Address, e.Flags)
	}
	e.Address = netip.AddrPortFrom(e.Address.Addr().Unmap(), e.Address.Port())
	e.LastSeen = time.Unix(e.LastSeen.Unix(), 0).UTC()
	i, ok := l.index(e.Address)
	if ok {
		l.Peers[i] = e
		return nil
	}
	if len(l.Peers) >= MaxPeerTableSize {
		return fmt.Errorf("the peer list is full, %d entries", MaxPeerTableSize)
	}
	l.Peers = slices.Insert(l.Peers, i, e)
	return nil
}

// RemovePeer removes the entry of address.
func (l *PeerList) RemovePeer(a netip.AddrPort) error {
	i, ok := l.index(a)
	if !ok {
		return fmt.Errorf("no peer %v", a)
	}
	l.Peers = slices.Delete(l.Peers, i, i+1)
	return nil
}

// Shareable returns the entries a get peers reply may hold at now: the
// shareable ones seen within PeerFreshAge, except the asker's own.
func (l *PeerList) Shareable(now time.Time, asker netip.AddrPort) []PeerEntry {
	var out []PeerEntry
	for _, e := range l.Peers {
		if e.Shareable() && e.fresh(now) && e.Address != asker {
			out = append(out, e)
		}
	}
	return out
}

// Learn adds the entries of a get peers reply as shareable, with their
// last-seen times, and returns how many were new. It skips an entry that
// is not shareable or not fresh, never changes a private entry, keeps the
// later last-seen time of an entry it has, and takes no new entry once the
// list is full, unless it is fresher than the stalest shareable entry,
// which it then replaces. A last-seen time in the future counts as now.
func (l *PeerList) Learn(entries []PeerEntry, now time.Time) int {
	added := 0
	for _, e := range entries {
		if e.LastSeen.After(now) {
			e.LastSeen = now
		}
		if !e.Shareable() || !e.fresh(now) || !e.Address.IsValid() || e.Address.Port() == 0 {
			continue
		}
		e.Flags = PeerShareable
		if have, ok := l.Peer(e.Address); ok {
			if have.Shareable() && e.LastSeen.After(have.LastSeen) {
				have.LastSeen = e.LastSeen
				l.SetPeer(have)
			}
			continue
		}
		if len(l.Peers) >= MaxPeerTableSize {
			stalest := -1
			for i, p := range l.Peers {
				if p.Shareable() && (stalest < 0 || p.LastSeen.Before(l.Peers[stalest].LastSeen)) {
					stalest = i
				}
			}
			if stalest < 0 || !e.LastSeen.After(l.Peers[stalest].LastSeen) {
				continue
			}
			l.Peers = slices.Delete(l.Peers, stalest, stalest+1)
		}
		if l.SetPeer(e) == nil {
			added++
		}
	}
	return added
}

// Clone returns a copy of l.
func (l *PeerList) Clone() *PeerList { return &PeerList{Peers: slices.Clone(l.Peers)} }

const peerListHeader = "[peers]"
const peerListColumns = "; address, last_seen, flags"

// Format returns the text form of the list: the [peers] header, a comment
// naming the columns, then one entry per line in address order. Last seen
// is Unix seconds and flags is the flag byte in decimal.
func (l *PeerList) Format() []byte {
	var b strings.Builder
	b.WriteString("; CX peer list, text form. Flags bit 0 marks a peer shareable; private peers are never sent or learned.\n")
	b.WriteString(peerListHeader + "\n" + peerListColumns + "\n")
	for _, e := range l.Peers {
		fmt.Fprintf(&b, "%s, %d, %d\n", e.Address, e.LastSeen.Unix(), e.Flags)
	}
	return []byte(b.String())
}

// ParsePeerList parses the text form of a peer list.
func ParsePeerList(text string) (*PeerList, error) {
	l := &PeerList{}
	section := ""
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, ";"):
			continue
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && !strings.Contains(line, ","):
			if line != peerListHeader {
				return nil, fmt.Errorf("line %d: unknown section %s, want %s", n+1, line, peerListHeader)
			}
			section = line
			continue
		case section == "":
			return nil, fmt.Errorf("line %d: entry before the %s header", n+1, peerListHeader)
		}
		f := strings.Split(line, ",")
		if len(f) != 3 {
			return nil, fmt.Errorf("line %d: want address, last_seen, flags", n+1)
		}
		a, err := ParsePeerAddress(strings.TrimSpace(f[0]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %v", n+1, err)
		}
		seen, err := strconv.ParseInt(strings.TrimSpace(f[1]), 10, 64)
		if err != nil || seen < 0 || seen > 0xFFFFFFFF {
			return nil, fmt.Errorf("line %d: last seen %q, want Unix seconds", n+1, strings.TrimSpace(f[1]))
		}
		flags, err := strconv.ParseUint(strings.TrimSpace(f[2]), 10, 8)
		if err != nil {
			return nil, fmt.Errorf("line %d: flags %q, want a byte", n+1, strings.TrimSpace(f[2]))
		}
		if _, dup := l.Peer(a); dup {
			return nil, fmt.Errorf("line %d: peer %v listed twice", n+1, a)
		}
		if err := l.SetPeer(PeerEntry{Address: a, LastSeen: time.Unix(seen, 0), Flags: byte(flags)}); err != nil {
			return nil, fmt.Errorf("line %d: %v", n+1, err)
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
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	return l, nil
}

// WritePeerList writes the list to path through a temporary file.
func WritePeerList(path string, l *PeerList) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, l.Format(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
