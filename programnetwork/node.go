package programnetwork

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// Node defaults, see "Flood replication" in docs/ProgramDistribution.md.
const (
	DefaultLease = 10 * time.Minute
	// DownloadAttempts is how many times a node tries to download a program
	// it heard of, cycling through the nodes that announced it.
	DownloadAttempts = 3
)

// ErrClosed is returned by requests of a node that was closed.
var ErrClosed = errors.New("node closed")

// Node is one node of the network. It holds programs in a programstore,
// serves them with the operations of programblocks, accepts subscribers,
// and floods every new program to them with a have packet, once.
type Node struct {
	// PacketSize bounds every packet the node sends, 1200 by default.
	PacketSize int
	// Forms selects the request forms of the node's downloads and get
	// peers, as in programblocks.Client.
	Forms programblocks.Forms
	// Lease is the longest subscription the node grants.
	Lease time.Duration
	// Logf, when set, receives one line per event.
	Logf func(format string, args ...any)
	// OnHeld, when set, is called once for every program the node starts
	// to hold, after it is verified and kept, before it is announced.
	OnHeld func(Held)
	// FirstTimeout, MaxTimeout and MaxTries set the retries of the node's
	// requests and downloads, as in programblocks.Client.
	FirstTimeout time.Duration
	MaxTimeout   time.Duration
	MaxTries     int

	conn     net.PacketConn
	store    *programstore.Store
	trusted  []cipher.PubKey
	settings Settings

	mu          sync.Mutex
	catalog     *programblocks.Catalog
	seen        map[programblocks.Hash]bool
	heard       map[programblocks.Hash]Have
	announcers  map[programblocks.Hash][]net.Addr
	announced   map[programblocks.Hash]bool
	subscribers map[string]subscriber
	subscribing map[string]bool
	pending     map[uint32]chan []byte
	nextID      uint32
	stats       Stats
	peers       *PeerList
	pins        pinList
	// reservedBytes are the bytes of the downloads under way, counted
	// against the budget with stats.StoredBytes.
	reservedBytes int64

	// storeMu serializes the writes to the store and its index, persistMu
	// the writes of subscriptions.txt and subscribers.txt.
	storeMu   sync.Mutex
	persistMu sync.Mutex
	saves     sync.WaitGroup
	stopping  bool // set under mu by Close
	closed    chan struct{}
	closeOnce sync.Once
}

type subscriber struct {
	addr    net.Addr
	expires time.Time
}

// Held is a program a node holds, verified.
type Held struct {
	Address  programblocks.Hash
	Manifest *programblocks.Manifest
	Files    map[string][]byte
	Name     string // the name in its cxapp.json
	From     string // the node it was downloaded from, "" when published here
	Elapsed  time.Duration
}

// Stats counts the flood traffic of a node.
type Stats struct {
	HavesReceived       int   // have packets accepted
	HavesDuplicate      int   // haves for a program already held or on its way
	HavesSent           int   // haves a subscriber acknowledged
	HavesUnacknowledged int   // haves given up after the retries
	Downloads           int   // programs downloaded and verified
	DownloadFailures    int   // programs given up after DownloadAttempts
	HavesIgnored        int   // haves of programs the node type does not keep
	HavesRefusedFull    int   // haves of programs that do not fit the storage
	Subscribers         int   // live subscribers
	StoredBytes         int64 // manifest and packing bytes of the programs held
}

// NewNode returns a super node, see NewTypedNode.
func NewNode(conn net.PacketConn, store *programstore.Store, trusted []cipher.PubKey) (*Node, error) {
	return NewTypedNode(conn, store, trusted, Settings{Type: TypeSuper})
}

// NewTypedNode returns a node of the type of settings that answers on conn
// and keeps programs in store. It downloads only programs signed by one of
// trusted. A medium or super node starts out holding every cached program
// of the store, with the subscribers of its subscribers.txt; a thin node
// holds nothing.
func NewTypedNode(conn net.PacketConn, store *programstore.Store, trusted []cipher.PubKey, settings Settings) (*Node, error) {
	if _, err := ParseNodeType(string(settings.Type)); err != nil {
		return nil, err
	}
	n := &Node{
		PacketSize: programblocks.DefaultPacketSize, Lease: DefaultLease,
		FirstTimeout: programblocks.DefaultFirstTimeout, MaxTimeout: programblocks.DefaultMaxTimeout, MaxTries: programblocks.DefaultMaxTries,
		conn: conn, store: store, trusted: trusted, settings: settings, pins: pinList{path: settings.PinsFile},
		catalog: programblocks.EmptyCatalog(), seen: map[programblocks.Hash]bool{}, heard: map[programblocks.Hash]Have{},
		announcers: map[programblocks.Hash][]net.Addr{}, announced: map[programblocks.Hash]bool{},
		subscribers: map[string]subscriber{}, subscribing: map[string]bool{}, pending: map[uint32]chan []byte{},
		nextID: rand.Uint32(), closed: make(chan struct{}),
	}
	if !settings.Type.keeps() {
		return n, nil
	}
	subs, err := readSubscribers(n.subscribersPath(), time.Now())
	if err != nil {
		return nil, err
	}
	n.subscribers = subs
	rows, err := store.ReadIndex()
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.State != programstore.StateCached {
			continue
		}
		addr, err := programblocks.HashFromHex(r.Address)
		if err != nil {
			return nil, err
		}
		m, err := store.Manifest(addr)
		if err != nil {
			return nil, err
		}
		files, err := store.Files(addr)
		if err != nil {
			return nil, err
		}
		encoded := m.Encode()
		if _, err := n.catalog.AddVerified(encoded, packFromFiles(m, files)); err != nil {
			return nil, fmt.Errorf("%s: %w", r.Name, err)
		}
		n.seen[addr] = true
		n.stats.StoredBytes += int64(len(encoded)) + int64(m.PackLength)
	}
	return n, nil
}

// Addr is the address the node answers on.
func (n *Node) Addr() net.Addr { return n.conn.LocalAddr() }

// Settings returns the node's type settings.
func (n *Node) Settings() Settings { return n.settings }

// Stats returns the node's counters.
func (n *Node) Stats() Stats {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.stats
	s.Subscribers = len(n.subscribers)
	return s
}

// Known returns a have for every program the node has heard of, held or
// not, in address order. A thin node picks what to Fetch from these.
func (n *Node) Known() []Have {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]Have, 0, len(n.heard))
	for _, h := range n.heard {
		out = append(out, h)
	}
	slices.SortFunc(out, func(a, b Have) int { return bytes.Compare(a.Address[:], b.Address[:]) })
	return out
}

// Holds reports whether the node holds the program at addr.
func (n *Node) Holds(addr programblocks.Hash) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.catalog.Holds(addr)
}

func (n *Node) logf(format string, args ...any) {
	if n.Logf != nil {
		n.Logf(format, args...)
	}
}

// Close stops the node and waits for its writes of subscribers.txt. Run
// returns.
func (n *Node) Close() error {
	n.closeOnce.Do(func() {
		n.mu.Lock()
		n.stopping = true
		n.mu.Unlock()
		close(n.closed)
	})
	err := n.conn.Close()
	n.saves.Wait()
	return err
}

// saveSubscribersLater writes subscribers.txt in the background, unless the
// node is closing. Close waits for it.
func (n *Node) saveSubscribersLater() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.stopping || !n.settings.Type.keeps() {
		return
	}
	n.saves.Add(1)
	go func() {
		defer n.saves.Done()
		n.saveSubscribers()
	}()
}

// Run answers packets until the node is closed.
func (n *Node) Run() error {
	if err := programblocks.CheckPacketSize(n.PacketSize); err != nil {
		return err
	}
	if n.settings.Type.keeps() {
		subs, err := n.Subscriptions()
		if err != nil {
			return err
		}
		for _, s := range subs {
			if err := n.startSubscription(s, true); err != nil {
				n.logf("subscribe to %s from %s: %v", s, SubscriptionsFile, err)
			}
		}
	}
	buf := make([]byte, 65536)
	for {
		size, from, err := n.conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-n.closed:
				return nil
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		n.handle(append([]byte(nil), buf[:size]...), from)
	}
}

func (n *Node) send(b []byte, to net.Addr) {
	if len(b) > n.PacketSize {
		n.logf("not sending %d bytes, over the packet size %d", len(b), n.PacketSize)
		return
	}
	n.conn.WriteTo(b, to)
}

func (n *Node) handle(b []byte, from net.Addr) {
	op, id, ok := PacketOperation(b)
	if !ok {
		return
	}
	fail := func(err error) {
		r, _ := programblocks.EncodeReply(programblocks.Reply{Operation: programblocks.OperationError, RequestID: id,
			ErrorCode: programblocks.ErrorCodeMalformedRequest, ErrorText: err.Error()}, n.PacketSize)
		n.send(r, from)
	}
	switch {
	case op&programblocks.ReplyFlag != 0:
		n.mu.Lock()
		ch := n.pending[id]
		n.mu.Unlock()
		if ch != nil {
			select {
			case ch <- b:
			default:
			}
		}
	case op == OperationHave:
		h, err := DecodeHave(b)
		if err != nil {
			fail(err)
			return
		}
		n.send(EncodeHaveReply(id, h.Address), from)
		n.onHave(h, from)
	case op == OperationSubscribe:
		lease, err := DecodeLease(b)
		if err != nil {
			fail(err)
			return
		}
		n.send(EncodeSubscribeReply(id, n.subscribe(from, lease)), from)
	case op == OperationGetPeers:
		first, err := DecodeGetPeers(b)
		if err != nil {
			fail(err)
			return
		}
		shared := n.sharedWith(from)
		var page []PeerEntry
		if int(first) < len(shared) {
			page = shared[first:]
		}
		r, _, err := EncodeGetPeersReply(id, uint16(len(shared)), first, page, n.PacketSize)
		if err != nil {
			fail(err)
			return
		}
		n.send(r, from)
	case op == OperationGetPeerPart:
		index, offset, err := DecodeGetPeerPart(b)
		if err != nil {
			fail(err)
			return
		}
		shared := n.sharedWith(from)
		var entry []byte
		if int(index) < len(shared) {
			if entry, err = EncodePeerEntry(shared[index]); err != nil {
				fail(err)
				return
			}
		}
		r, err := EncodeGetPeerPartReply(id, uint16(len(shared)), index, entry, offset, n.PacketSize)
		if err != nil {
			fail(err)
			return
		}
		n.send(r, from)
	default:
		n.mu.Lock()
		r := n.catalog.Reply(b, n.PacketSize)
		n.mu.Unlock()
		if r != nil {
			n.send(r, from)
		}
	}
}

// subscribe records or renews a subscriber and returns the lease granted.
// A lease of 0 ends the subscription. A new subscriber is sent a have for
// every program held (D34). A thin node grants no lease, since it never
// announces.
func (n *Node) subscribe(from net.Addr, leaseSeconds uint32) uint32 {
	if !n.settings.Type.keeps() {
		return 0
	}
	lease := min(time.Duration(leaseSeconds)*time.Second, n.Lease)
	key := from.String()
	n.mu.Lock()
	_, known := n.subscribers[key]
	defer n.saveSubscribersLater()
	if lease <= 0 {
		delete(n.subscribers, key)
		n.mu.Unlock()
		return 0
	}
	n.subscribers[key] = subscriber{addr: from, expires: time.Now().Add(lease)}
	var haves []Have
	if !known {
		for _, addr := range n.catalog.Addresses() {
			haves = append(haves, n.haveLocked(addr))
		}
	}
	n.mu.Unlock()
	if !known {
		n.logf("subscriber %s, %d programs to announce", key, len(haves))
		go func() {
			for _, h := range haves {
				n.sendHave(from, h)
			}
		}()
	}
	return uint32(lease / time.Second)
}

func (n *Node) haveLocked(addr programblocks.Hash) Have {
	encoded, pack, _ := n.catalog.Item(addr)
	return Have{Address: addr, ManifestLength: uint32(len(encoded)), PackLength: uint64(len(pack))}
}

func (n *Node) newRequestID() uint32 {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.nextID++
	return n.nextID
}

// request sends packet, whose request ID is id, to the node at to, and
// waits for its reply, sending it again after each timeout.
func (n *Node) request(to net.Addr, id uint32, packet []byte) ([]byte, error) {
	ch := make(chan []byte, 1)
	n.mu.Lock()
	n.pending[id] = ch
	n.mu.Unlock()
	defer func() {
		n.mu.Lock()
		delete(n.pending, id)
		n.mu.Unlock()
	}()
	timeout := n.FirstTimeout
	for try := 1; try <= n.MaxTries; try++ {
		n.send(packet, to)
		t := time.NewTimer(timeout)
		select {
		case r := <-ch:
			t.Stop()
			if programblocks.Operation(r[1]) == programblocks.OperationError {
				rep, err := programblocks.DecodeReply(r)
				if err != nil {
					return nil, err
				}
				return nil, &programblocks.ReplyError{Code: rep.ErrorCode, Text: rep.ErrorText}
			}
			if programblocks.Operation(r[1]) != programblocks.Operation(packet[1])|programblocks.ReplyFlag {
				return nil, fmt.Errorf("%w: reply %#x to operation %#x", programblocks.ErrPacket, r[1], packet[1])
			}
			return r, nil
		case <-t.C:
		case <-n.closed:
			t.Stop()
			return nil, ErrClosed
		}
		timeout = min(2*timeout, n.MaxTimeout)
	}
	return nil, fmt.Errorf("no reply from %s to operation %#x after %d tries", to, packet[1], n.MaxTries)
}

// Subscribe asks the node at address for have packets, in the background:
// it tries until the node replies, then renews at half the lease, until
// this node is closed. Only an address that does not resolve is an error.
// A medium or super node adds address to its subscriptions.txt and
// subscribes again after a restart. A second call for one node does
// nothing.
func (n *Node) Subscribe(address string) error { return n.startSubscription(address, false) }

// startSubscription runs a subscription. A resumed one, from
// subscriptions.txt after a restart, starts fresh, see subscribeOnce.
func (n *Node) startSubscription(address string, resumed bool) error {
	to, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	n.mu.Lock()
	already := n.subscribing[to.String()]
	n.subscribing[to.String()] = true
	n.mu.Unlock()
	if already {
		return nil
	}
	if err := n.rememberSubscription(address); err != nil {
		n.logf("save %s: %v", SubscriptionsFile, err)
	}
	go func() {
		var lease uint32
		for {
			l, err := n.subscribeOnce(to, resumed && lease == 0)
			wait := time.Second
			switch {
			case errors.Is(err, ErrClosed):
				return
			case err != nil:
				n.logf("subscribe to %s: %v, trying again", address, err)
			default:
				if lease == 0 {
					n.logf("subscribed to %s for %d s", address, l)
				}
				lease = l
				wait = time.Duration(max(lease, 2)) * time.Second / 2
			}
			select {
			case <-n.closed:
				return
			case <-time.After(wait):
			}
		}
	}()
	return nil
}

// subscribeOnce asks for a lease. A fresh subscription ends any earlier
// one first, so a node that restarted, which the other node may still
// count as a subscriber, gets the catch-up haves of D34.
func (n *Node) subscribeOnce(to net.Addr, fresh bool) (uint32, error) {
	if fresh {
		id := n.newRequestID()
		if _, err := n.request(to, id, EncodeSubscribe(id, 0)); err != nil {
			return 0, err
		}
	}
	id := n.newRequestID()
	r, err := n.request(to, id, EncodeSubscribe(id, uint32(n.Lease/time.Second)))
	if err != nil {
		return 0, err
	}
	return DecodeLease(r)
}

// GetPeers asks the node at address for every peer it shares with this
// node, a page at a time.
func (n *Node) GetPeers(address string) ([]PeerEntry, error) {
	to, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	var all []PeerEntry
	total := -1
	for total < 0 || len(all) < total {
		if n.Forms != programblocks.FormsShort {
			id := n.newRequestID()
			b, err := n.request(to, id, EncodeGetPeers(id, uint16(len(all))))
			if err != nil {
				return nil, err
			}
			r, err := DecodeGetPeersReply(b)
			if err != nil {
				return nil, err
			}
			if int(r.First) != len(all) {
				return nil, fmt.Errorf("%w: get peers page at %d, asked %d", programblocks.ErrPacket, r.First, len(all))
			}
			total = int(r.Total)
			all = append(all, r.Entries...)
			if len(r.Entries) == 0 && len(all) < total {
				return all, programblocks.PacketTooSmall(n.PacketSize, n.Forms, MaxPeerEntryLength+getPeersReplyFixed, "peer %d in a get peers reply", len(all))
			}
			continue
		}
		e, t, err := n.getPeerByParts(to, uint16(len(all)))
		if err != nil {
			return all, err
		}
		if total = t; e != nil {
			all = append(all, *e)
		}
	}
	return all, nil
}

// getPeerByParts reads the peer entry at index with get peer part, a range
// at a time. It returns nil and the total when index is past the total.
func (n *Node) getPeerByParts(to net.Addr, index uint16) (*PeerEntry, int, error) {
	var entry []byte
	for offset := 0; entry == nil || offset < len(entry); {
		id := n.newRequestID()
		b, err := n.request(to, id, EncodeGetPeerPart(id, index, offset))
		if err != nil {
			return nil, 0, err
		}
		p, err := DecodeGetPeerPartReply(b)
		if err != nil {
			return nil, 0, err
		}
		if p.Index != index || p.Offset != offset || (entry != nil && p.EntryLength != len(entry)) {
			return nil, 0, fmt.Errorf("%w: get peer part of entry %d at %d, asked %d at %d", programblocks.ErrPacket, p.Index, p.Offset, index, offset)
		}
		if p.EntryLength == 0 {
			return nil, int(p.Total), nil
		}
		if entry == nil {
			entry = make([]byte, p.EntryLength)
		}
		offset += copy(entry[offset:], p.Data)
		if offset == len(entry) {
			e, err := DecodePeerEntry(entry)
			return &e, int(p.Total), err
		}
	}
	return nil, 0, nil
}

// sharedWith returns the peers this node shares with the asker at from.
func (n *Node) sharedWith(from net.Addr) []PeerEntry {
	n.mu.Lock()
	peers := n.peers
	n.mu.Unlock()
	if peers == nil {
		return nil
	}
	return peers.SharedWith(from.String())
}

// sendHave announces h to one subscriber and waits for the acknowledgement.
func (n *Node) sendHave(to net.Addr, h Have) {
	id := n.newRequestID()
	_, err := n.request(to, id, EncodeHave(id, h))
	n.mu.Lock()
	defer n.mu.Unlock()
	if err != nil {
		n.stats.HavesUnacknowledged++
		n.logf("have %s to %s: %v", h.Address, to, err)
		return
	}
	n.stats.HavesSent++
}

// announce sends a have for addr to every live subscriber, once per
// program, which is what ends a flood.
func (n *Node) announce(addr programblocks.Hash) {
	n.mu.Lock()
	if n.announced[addr] {
		n.mu.Unlock()
		return
	}
	n.announced[addr] = true
	h := n.haveLocked(addr)
	var to []net.Addr
	now := time.Now()
	expired := false
	for key, s := range n.subscribers {
		if now.After(s.expires) {
			delete(n.subscribers, key)
			expired = true
			continue
		}
		to = append(to, s.addr)
	}
	n.mu.Unlock()
	if expired {
		n.saveSubscribersLater()
	}
	n.logf("announce %s to %d subscribers", addr, len(to))
	for _, a := range to {
		go n.sendHave(a, h)
	}
}

// size is the bytes a program of the have takes in a store.
func (h Have) size() int64 { return int64(h.ManifestLength) + int64(h.PackLength) }

// onHave records who announced a program and starts its download when the
// node type keeps it: a thin node never, a medium node when it is pinned
// or fits the budget, a super node when it fits the capacity.
func (n *Node) onHave(h Have, from net.Addr) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stats.HavesReceived++
	n.heard[h.Address] = h
	known := false
	for _, a := range n.announcers[h.Address] {
		known = known || a.String() == from.String()
	}
	if !known {
		n.announcers[h.Address] = append(n.announcers[h.Address], from)
	}
	if n.seen[h.Address] {
		n.stats.HavesDuplicate++
		return
	}
	fits := n.settings.Budget == 0 || n.stats.StoredBytes+n.reservedBytes+h.size() <= n.settings.Budget
	switch n.settings.Type {
	case TypeThin:
		n.stats.HavesIgnored++
		return
	case TypeMedium:
		if !n.pins.pinned(h.Address, n.logf) && (n.settings.Budget == 0 || !fits) {
			n.stats.HavesIgnored++
			n.logf("have %s from %s, not pinned and no room in the budget for %d bytes", h.Address, from, h.size())
			return
		}
	case TypeSuper:
		if !fits {
			n.stats.HavesRefusedFull++
			n.logf("storage full, not taking %s of %d bytes, %d of %d bytes held", h.Address, h.size(), n.stats.StoredBytes, n.settings.Budget)
			return
		}
	}
	n.seen[h.Address] = true
	n.reservedBytes += h.size()
	n.logf("have %s from %s, %d manifest bytes, %d packing bytes", h.Address, from, h.ManifestLength, h.PackLength)
	go n.download(h)
}

// download fetches a program from the nodes that announced it, verifies
// it, keeps it and announces it in turn. A program from an untrusted
// signer, or one whose lengths differ from the have, is dropped for good.
// A full disk drops the partial program. Any other failure lets a later
// have start over.
func (n *Node) download(h Have) {
	defer func() {
		n.mu.Lock()
		n.reservedBytes -= h.size()
		n.mu.Unlock()
	}()
	for attempt := range DownloadAttempts {
		n.mu.Lock()
		list := n.announcers[h.Address]
		from := list[attempt%len(list)]
		n.mu.Unlock()
		p, encoded, err := n.fetchFrom(h, from)
		if err == nil {
			err = n.hold(p, encoded)
		}
		if err == nil {
			return
		}
		n.logf("download %s from %s: %v", h.Address, from, err)
		if errors.Is(err, syscall.ENOSPC) {
			n.dropFull(h.Address)
			return
		}
		if errors.Is(err, programblocks.ErrUntrusted) || errors.Is(err, errHaveMismatch) {
			return
		}
	}
	n.mu.Lock()
	n.stats.DownloadFailures++
	delete(n.seen, h.Address)
	n.mu.Unlock()
}

// dropFull removes what a download that filled the disk left in the store,
// so the node keeps serving what it held before.
func (n *Node) dropFull(addr programblocks.Hash) {
	n.storeMu.Lock()
	err := n.store.Remove(addr)
	n.storeMu.Unlock()
	n.mu.Lock()
	n.stats.HavesRefusedFull++
	delete(n.seen, addr)
	n.mu.Unlock()
	n.logf("storage full, dropped the partial %s: %v", addr, err)
}

var errHaveMismatch = errors.New("manifest lengths differ from the have")

// fetchFrom downloads the program of h from the node at from into the
// store and returns it verified, without holding it. A have with zero
// lengths, as Fetch makes, skips the length check.
func (n *Node) fetchFrom(h Have, from net.Addr) (Held, []byte, error) {
	start := time.Now()
	conn, err := net.Dial("udp", from.String())
	if err != nil {
		return Held{}, nil, err
	}
	defer conn.Close()
	c := programblocks.NewClient(conn, n.trusted...)
	c.PacketSize, c.FirstTimeout, c.MaxTimeout, c.MaxTries = n.PacketSize, n.FirstTimeout, n.MaxTimeout, n.MaxTries
	c.Forms = n.Forms
	m, encoded, err := c.FetchManifest(h.Address)
	if err != nil {
		return Held{}, nil, err
	}
	if h.ManifestLength != 0 && (uint32(len(encoded)) != h.ManifestLength || m.PackLength != h.PackLength) {
		return Held{}, nil, fmt.Errorf("%w: %d and %d bytes, have says %d and %d", errHaveMismatch, len(encoded), m.PackLength, h.ManifestLength, h.PackLength)
	}
	n.storeMu.Lock()
	defer n.storeMu.Unlock()
	if _, _, err := n.store.SaveManifest(encoded, n.trusted); err != nil {
		return Held{}, nil, err
	}
	if err := n.setRow(h.Address, programstore.StateRemote, m.PackLength, h.Address.String()[:16]); err != nil {
		return Held{}, nil, err
	}
	files, err := n.store.Download(c, h.Address)
	if err != nil {
		return Held{}, nil, err
	}
	name := programName(files, h.Address)
	if err := n.setRow(h.Address, programstore.StateCached, m.PackLength, name); err != nil {
		return Held{}, nil, err
	}
	n.mu.Lock()
	n.stats.Downloads++
	n.mu.Unlock()
	return Held{Address: h.Address, Manifest: m, Files: files, Name: name, From: from.String(), Elapsed: time.Since(start)}, encoded, nil
}

// Fetch returns the program at addr, verified: from the store when it is
// cached there, or else downloaded from the nodes given and then the ones
// that announced it. A thin node keeps the program in its store for
// itself only and neither serves nor announces it; a medium or super node
// holds and announces it, pinned or not and over its budget or not.
func (n *Node) Fetch(addr programblocks.Hash, from ...string) (Held, error) {
	n.storeMu.Lock()
	row, err := n.store.FindRow(addr.String())
	n.storeMu.Unlock()
	if err == nil && row.State == programstore.StateCached {
		if m, err := n.store.Manifest(addr); err == nil {
			if files, err := n.store.Files(addr); err == nil {
				p := Held{Address: addr, Manifest: m, Files: files, Name: row.Name}
				return p, n.keepFetched(p, m.Encode())
			}
		}
	}
	var sources []net.Addr
	for _, f := range from {
		a, err := net.ResolveUDPAddr("udp", f)
		if err != nil {
			return Held{}, err
		}
		sources = append(sources, a)
	}
	n.mu.Lock()
	sources = append(sources, n.announcers[addr]...)
	h, ok := n.heard[addr]
	n.mu.Unlock()
	if !ok {
		h = Have{Address: addr}
	}
	if len(sources) == 0 {
		return Held{}, fmt.Errorf("no node is known to hold %s", addr)
	}
	var last error
	for attempt := range max(DownloadAttempts, len(sources)) {
		from := sources[attempt%len(sources)]
		p, encoded, err := n.fetchFrom(h, from)
		if err == nil {
			return p, n.keepFetched(p, encoded)
		}
		last = fmt.Errorf("fetch %s from %s: %w", addr, from, err)
		n.logf("%v", last)
		if errors.Is(err, programblocks.ErrUntrusted) || errors.Is(err, errHaveMismatch) {
			break
		}
	}
	return Held{}, last
}

// keepFetched holds a fetched program on a medium or super node.
func (n *Node) keepFetched(p Held, encoded []byte) error {
	if !n.settings.Type.keeps() || n.Holds(p.Address) {
		return nil
	}
	return n.hold(p, encoded)
}

func programName(files map[string][]byte, addr programblocks.Hash) string {
	if app, err := programblocks.AppManifest(files); err == nil && app.Name != "" {
		return app.Name
	}
	return addr.String()[:16]
}

// hold adds a verified program to the catalog, tells OnHeld and announces
// it.
func (n *Node) hold(p Held, encoded []byte) error {
	n.mu.Lock()
	already := n.catalog.Holds(p.Address)
	_, err := n.catalog.AddVerified(encoded, packFromFiles(p.Manifest, p.Files))
	n.seen[p.Address] = true
	if err == nil && !already {
		n.stats.StoredBytes += int64(len(encoded)) + int64(p.Manifest.PackLength)
	}
	n.mu.Unlock()
	if err != nil {
		return err
	}
	n.logf("holding %s %s from %q in %d ms", p.Name, p.Address, p.From, p.Elapsed.Milliseconds())
	if n.OnHeld != nil {
		n.OnHeld(p)
	}
	n.announce(p.Address)
	return nil
}

// setRow adds or updates the row of addr in the store's index.txt.
func (n *Node) setRow(addr programblocks.Hash, state string, length uint64, name string) error {
	rows, err := n.store.ReadIndex()
	if err != nil {
		return err
	}
	hex := addr.String()
	for i := range rows {
		if rows[i].Address == hex {
			rows[i].State, rows[i].Name = state, name
			return n.store.WriteIndex(rows)
		}
	}
	rows = append(rows, programstore.Row{Number: len(rows) + 1, Address: hex, State: state, Length: int64(length), Name: name})
	return n.store.WriteIndex(rows)
}

// Publish signs files as a new program with secret, keeps it, and floods
// it to the subscribers. The signer must be one of the node's trusted keys.
// A thin node does not publish, since it serves nothing.
func (n *Node) Publish(files map[string][]byte, secret cipher.SecKey, date time.Time) (programblocks.Hash, error) {
	if !n.settings.Type.keeps() {
		return programblocks.Hash{}, errors.New("a thin node does not publish")
	}
	m, pack, err := programblocks.Build(files, programblocks.DefaultBlockSize, date)
	if err != nil {
		return programblocks.Hash{}, err
	}
	if err := m.Sign(secret); err != nil {
		return programblocks.Hash{}, err
	}
	encoded := m.Encode()
	addr := m.Address()
	n.storeMu.Lock()
	err = n.keepPublished(addr, m, encoded, pack, files)
	n.storeMu.Unlock()
	if err != nil {
		return addr, err
	}
	return addr, n.hold(Held{Address: addr, Manifest: m, Files: files, Name: programName(files, addr)}, encoded)
}

func (n *Node) keepPublished(addr programblocks.Hash, m *programblocks.Manifest, encoded, pack []byte, files map[string][]byte) error {
	if _, _, err := n.store.SaveManifest(encoded, n.trusted); err != nil {
		return err
	}
	p, err := n.store.CreatePacking(addr, m)
	if err != nil {
		return err
	}
	_, err = p.Assembly.Accept(0, pack)
	if cerr := p.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return n.setRow(addr, programstore.StateCached, m.PackLength, programName(files, addr))
}

// packFromFiles lays the files of m out as its linear packing.
func packFromFiles(m *programblocks.Manifest, files map[string][]byte) []byte {
	pack := make([]byte, m.PackLength)
	for _, f := range m.Files {
		copy(pack[f.PackOffset:], files[f.Path])
	}
	return pack
}

// SetPeers sets the peer list get peers answers from. The node does not
// change the list. With no list it shares no peer.
func (n *Node) SetPeers(l *PeerList) {
	n.mu.Lock()
	n.peers = l
	n.mu.Unlock()
}

// Store is the store the node keeps its programs in.
func (n *Node) Store() *programstore.Store { return n.store }
