package programnetwork

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// NodeType decides what a node downloads, keeps, announces and serves, see
// "Node types (#34)" in docs/ProgramDistribution.md.
type NodeType string

// The node types.
const (
	// TypeThin fetches what it needs on demand, keeps it only for itself,
	// and never announces or serves a program.
	TypeThin NodeType = "thin"
	// TypeMedium keeps the pinned programs and others within its budget,
	// and serves and announces what it keeps.
	TypeMedium NodeType = "medium"
	// TypeSuper keeps every program it hears of until its storage is full.
	TypeSuper NodeType = "super"
)

// ParseNodeType parses thin, medium or super.
func ParseNodeType(s string) (NodeType, error) {
	switch t := NodeType(s); t {
	case TypeThin, TypeMedium, TypeSuper:
		return t, nil
	}
	return "", fmt.Errorf("node type %q, want thin, medium or super", s)
}

// keeps reports whether the type keeps programs for others.
func (t NodeType) keeps() bool { return t != TypeThin }

// Settings are the type settings of a node.
type Settings struct {
	Type NodeType
	// Budget is the most bytes of manifests and packings a medium node
	// keeps, and the capacity of a super node. 0 is no budget: a medium
	// node then keeps only pinned programs, a super node keeps programs
	// until a write fails.
	Budget int64
	// PinsFile is a medium node's pin list file, read again whenever it
	// changes, so `programnetwork pins add` takes effect at once.
	PinsFile string
}

// The pin length bounds in hex digits: 4 to 32 bytes, as the prefixes of
// the short forms (D36).
const (
	MinPinDigits = 2 * programblocks.MinPrefixLength
	MaxPinDigits = 64
)

// ParsePin checks a pin, a manifest address or a prefix of one in hex, and
// returns it in lower case.
func ParsePin(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < MinPinDigits || len(s) > MaxPinDigits {
		return "", fmt.Errorf("pin %q: want %d to %d hex digits", s, MinPinDigits, MaxPinDigits)
	}
	if _, err := hex.DecodeString(s + strings.Repeat("0", len(s)%2)); err != nil {
		return "", fmt.Errorf("pin %q: not hex", s)
	}
	return s, nil
}

// ReadPins reads a pin list file: one address or prefix per line, # starts
// a comment line. A missing file has no pins.
func ReadPins(path string) ([]string, error) {
	lines, err := programstore.ReadLines(path)
	if err != nil {
		return nil, err
	}
	pins := make([]string, 0, len(lines))
	for i, l := range lines {
		p, err := ParsePin(strings.Fields(l)[0])
		if err != nil {
			return nil, fmt.Errorf("%s entry %d: %v", path, i+1, err)
		}
		pins = append(pins, p)
	}
	return pins, nil
}

// WritePins writes pins sorted and without repeats, one per line.
func WritePins(path string, pins []string) error {
	pins = slices.Clone(pins)
	slices.Sort(pins)
	return programstore.WriteLines(path, slices.Compact(pins))
}

// pinList is a pin list file, read again when its modification time or
// size changes.
type pinList struct {
	path  string
	mod   time.Time
	size  int64
	pins  []string
	known bool
}

// pinned reports whether a pin matches addr, reading the file again first
// when it changed. A file that does not parse pins nothing and is logged.
func (l *pinList) pinned(addr programblocks.Hash, logf func(string, ...any)) bool {
	if l.path == "" {
		return false
	}
	st, err := os.Stat(l.path)
	switch {
	case err != nil:
		l.pins, l.known = nil, false
	case !l.known || !st.ModTime().Equal(l.mod) || st.Size() != l.size:
		l.mod, l.size, l.known = st.ModTime(), st.Size(), true
		if l.pins, err = ReadPins(l.path); err != nil {
			logf("pins: %v", err)
		}
	}
	hex := addr.String()
	for _, p := range l.pins {
		if strings.HasPrefix(hex, p) {
			return true
		}
	}
	return false
}

// The files a medium or super node keeps next to its store's index.txt.
const (
	// SubscriptionsFile lists the nodes this node subscribes to, one
	// HOST:PORT per line.
	SubscriptionsFile = "subscriptions.txt"
	// SubscribersFile lists the subscribers of this node, one
	// "HOST:PORT EXPIRES" per line, EXPIRES in RFC 3339.
	SubscribersFile = "subscribers.txt"
)

func (n *Node) subscriptionsPath() string { return filepath.Join(n.store.Dir, SubscriptionsFile) }
func (n *Node) subscribersPath() string   { return filepath.Join(n.store.Dir, SubscribersFile) }

// readSubscribers returns the unexpired subscribers of subscribers.txt.
func readSubscribers(path string, now time.Time) (map[string]subscriber, error) {
	lines, err := programstore.ReadLines(path)
	if err != nil {
		return nil, err
	}
	out := map[string]subscriber{}
	for i, l := range lines {
		f := strings.Fields(l)
		if len(f) != 2 {
			return nil, fmt.Errorf("%s entry %d: want HOST:PORT EXPIRES", path, i+1)
		}
		addr, err := net.ResolveUDPAddr("udp", f[0])
		if err != nil {
			return nil, fmt.Errorf("%s entry %d: %v", path, i+1, err)
		}
		expires, err := time.Parse(time.RFC3339, f[1])
		if err != nil {
			return nil, fmt.Errorf("%s entry %d: %v", path, i+1, err)
		}
		if expires.After(now) {
			out[addr.String()] = subscriber{addr: addr, expires: expires}
		}
	}
	return out, nil
}

// subscriberLinesLocked formats the subscribers as subscribers.txt, sorted.
func (n *Node) subscriberLinesLocked() []string {
	lines := make([]string, 0, len(n.subscribers))
	for key, s := range n.subscribers {
		lines = append(lines, key+" "+s.expires.UTC().Format(time.RFC3339))
	}
	slices.Sort(lines)
	return lines
}

// saveSubscribers writes subscribers.txt for a node that keeps programs.
// Writes are serialized, and each writes the list as it is then.
func (n *Node) saveSubscribers() {
	if !n.settings.Type.keeps() {
		return
	}
	n.persistMu.Lock()
	defer n.persistMu.Unlock()
	n.mu.Lock()
	lines := n.subscriberLinesLocked()
	n.mu.Unlock()
	if err := programstore.WriteLines(n.subscribersPath(), lines); err != nil {
		n.logf("save %s: %v", SubscribersFile, err)
	}
}

// rememberSubscription adds address to subscriptions.txt for a node that
// keeps programs.
func (n *Node) rememberSubscription(address string) error {
	if !n.settings.Type.keeps() {
		return nil
	}
	n.persistMu.Lock()
	defer n.persistMu.Unlock()
	lines, err := programstore.ReadLines(n.subscriptionsPath())
	if err != nil || slices.Contains(lines, address) {
		return err
	}
	return programstore.WriteLines(n.subscriptionsPath(), append(lines, address))
}

// Subscriptions returns the nodes of subscriptions.txt.
func (n *Node) Subscriptions() ([]string, error) {
	return programstore.ReadLines(n.subscriptionsPath())
}
