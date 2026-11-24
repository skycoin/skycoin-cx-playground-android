package programnetwork

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// Node defaults, see "Announce and request" in docs/ProgramDistribution.md.
const (
	DefaultLease = 10 * time.Minute
	// DownloadAttempts is how many times a node tries a download it
	// requested, cycling through the nodes that announced the program.
	DownloadAttempts = 3
)

// ErrClosed is returned by requests of a node that was closed.
var ErrClosed = errors.New("node closed")

// Node is a medium or super node of the network (#44). It holds programs
// in a programstore, serves them with the operations of programblocks,
// keeps a persistent subscription to its peers, and puts every have it
// receives in its have queue (#42). A have never starts a download: Update
// reads the queue and the node type's policy requests what it keeps. A
// node relays a have to its subscribers once, when it holds the program
// (D55). A thin node is Get, not a Node.
type Node struct {
	// PacketSize bounds every packet the node sends, 1200 by default.
	PacketSize int
	// Forms is the forms flag of the node's downloads, as in
	// programblocks.Client.
	Forms programblocks.Forms
	// Lease is the longest subscription the node grants.
	Lease time.Duration
	// Logf, when set, receives one line per event.
	Logf func(format string, args ...any)
	// OnHeld, when set, is called once for every program the node starts
	// to hold, after it is verified and kept, before it is relayed.
	OnHeld func(Held)
	// FirstTimeout, MaxTimeout and MaxTries set the retries of the node's
	// requests and downloads, as in programblocks.Client.
	FirstTimeout time.Duration
	MaxTimeout   time.Duration
	MaxTries     int
	// Now, when set, is the node's clock for peer exchange, for tests.
	Now func() time.Time

	conn     net.PacketConn
	store    *programstore.Store
	trusted  []cipher.PubKey
	settings Settings
	haves    *HaveQueue

	mu          sync.Mutex
	catalog     *programblocks.Catalog
	requested   map[programblocks.Hash]bool
	announcers  map[programblocks.Hash][]net.Addr
	relayed     map[programblocks.Hash]bool
	subscribers map[string]subscriber
	subscribing map[string]bool
	pending     map[uint32]chan []byte
	nextID      uint32
	stats       Stats
	peers       *PeerList
	peerAnswers map[string]peerAnswer
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

// Stats counts the announce and request traffic of a node.
type Stats struct {
	HavesReceived       int   // have packets put in the have queue
	HavesRead           int   // haves the program read from the queue
	HavesDuplicate      int   // haves read for a program held or requested
	HavesSent           int   // haves a subscriber acknowledged
	HavesUnacknowledged int   // haves given up after the retries
	Requests            int   // downloads the node's policy requested
	Downloads           int   // programs downloaded and verified
	DownloadFailures    int   // requests given up after DownloadAttempts
	HavesRefusedFull    int   // haves of programs that do not fit the storage
	Subscribers         int   // live subscribers
	StoredBytes         int64 // manifest and packing bytes of the programs held
}

// NewNode returns a super node, see NewTypedNode.
func NewNode(conn net.PacketConn, store *programstore.Store, trusted []cipher.PubKey) (*Node, error) {
	return NewTypedNode(conn, store, trusted, Settings{Type: TypeSuper})
}

// NewTypedNode returns a medium or super node that answers on conn and
// keeps programs in store. It downloads only programs signed by one of
// trusted. It starts out holding every cached program of the store, with
// the subscribers of its subscribers.txt.
func NewTypedNode(conn net.PacketConn, store *programstore.Store, trusted []cipher.PubKey, settings Settings) (*Node, error) {
	if _, err := ParseNodeType(string(settings.Type)); err != nil {
		return nil, err
	}
	if settings.Type == TypeThin {
		return nil, errors.New("a thin node is a one-shot get, not a running node")
	}
	n := &Node{
		PacketSize: programblocks.DefaultPacketSize, Lease: DefaultLease,
		FirstTimeout: programblocks.DefaultFirstTimeout, MaxTimeout: programblocks.DefaultMaxTimeout, MaxTries: programblocks.DefaultMaxTries,
		conn: conn, store: store, trusted: trusted, settings: settings, pins: pinList{path: settings.PinsFile}, haves: newHaveQueue(),
		catalog: programblocks.EmptyCatalog(), requested: map[programblocks.Hash]bool{},
		announcers: map[programblocks.Hash][]net.Addr{}, relayed: map[programblocks.Hash]bool{},
		subscribers: map[string]subscriber{}, subscribing: map[string]bool{}, pending: map[uint32]chan []byte{}, peerAnswers: map[string]peerAnswer{},
		nextID: rand.Uint32(), closed: make(chan struct{}),
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
		n.relayed[addr] = true
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

// Haves is the node's have queue.
func (n *Node) Haves() *HaveQueue { return n.haves }

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
	if n.stopping {
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
	subs, err := n.Subscriptions()
	if err != nil {
		return err
	}
	for _, s := range subs {
		if err := n.startSubscription(s, true); err != nil {
			n.logf("subscribe to %s from %s: %v", s, SubscriptionsFile, err)
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
		if err := DecodeGetPeers(b); err != nil {
			fail(err)
			return
		}
		n.send(n.peersReply(id, from), from)
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
// every program held (D34). A node that does not relay grants no lease.
func (n *Node) subscribe(from net.Addr, leaseSeconds uint32) uint32 {
	if !n.settings.relays() {
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

// Subscribe keeps a persistent subscription to the node at address, in
// the background: it tries until the node replies, then renews at half the
// lease, until this node is closed, and every have the node sends goes in
// the have queue. Only an address that does not resolve is an error. The
// node adds address to its subscriptions.txt and subscribes again after a
// restart. A second call for one node does nothing.
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
				if l == 0 {
					n.logf("%s grants no subscription, it does not relay", address)
					return
				}
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

// GetPeers asks the node at address for peers, as Bitcoin's getaddr, and
// learns the shareable, fresh entries of the reply into the node's peer
// list (#41). It returns the entries the reply held.
func (n *Node) GetPeers(address string) ([]PeerEntry, error) {
	to, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	id := n.newRequestID()
	b, err := n.request(to, id, EncodeGetPeers(id))
	if err != nil {
		return nil, err
	}
	entries, err := DecodeGetPeersReply(b)
	if err != nil {
		return nil, err
	}
	now := n.now()
	n.mu.Lock()
	if n.peers == nil {
		n.peers = &PeerList{}
	}
	added := n.peers.Learn(entries, now)
	if from, ok := addrPort(to); ok {
		if e, ok := n.peers.Peer(from); ok && e.Shareable() {
			e.LastSeen = now
			n.peers.SetPeer(e)
		}
	}
	n.mu.Unlock()
	n.logf("get peers from %s: %d entries, %d new", address, len(entries), added)
	return entries, nil
}

// peerAnswer is the last get peers reply a node sent one asker.
type peerAnswer struct {
	at        time.Time
	requestID uint32
	reply     []byte
}

// peersReply answers a get peers. An asker gets entries at most once per
// PeerAnswerInterval; a retransmission of that request gets the same reply,
// and any other request within the interval a reply with no entries. The
// entries are the fresh shareable ones in random order, as Bitcoin picks.
func (n *Node) peersReply(id uint32, from net.Addr) []byte {
	now := n.now()
	key := from.String()
	n.mu.Lock()
	defer n.mu.Unlock()
	for k, a := range n.peerAnswers {
		if now.Sub(a.at) >= PeerAnswerInterval {
			delete(n.peerAnswers, k)
		}
	}
	if a, ok := n.peerAnswers[key]; ok {
		if a.requestID == id {
			return a.reply
		}
		r, _ := EncodeGetPeersReply(id, nil, n.PacketSize)
		return r
	}
	var shared []PeerEntry
	if n.peers != nil {
		asker, _ := addrPort(from)
		shared = n.peers.Shareable(now, asker)
	}
	rand.Shuffle(len(shared), func(i, j int) { shared[i], shared[j] = shared[j], shared[i] })
	r, _ := EncodeGetPeersReply(id, shared, n.PacketSize)
	n.peerAnswers[key] = peerAnswer{at: now, requestID: id, reply: r}
	return r
}

func addrPort(a net.Addr) (netip.AddrPort, bool) {
	u, ok := a.(*net.UDPAddr)
	if !ok {
		return netip.AddrPort{}, false
	}
	p := u.AddrPort()
	return netip.AddrPortFrom(p.Addr().Unmap(), p.Port()), true
}

func (n *Node) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
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

// relay sends a have for addr to every live subscriber, once per program
// (D55), when the node relays.
func (n *Node) relay(addr programblocks.Hash) {
	n.mu.Lock()
	if n.relayed[addr] || !n.settings.relays() {
		n.mu.Unlock()
		return
	}
	n.relayed[addr] = true
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
	n.logf("relay %s to %d subscribers", addr, len(to))
	for _, a := range to {
		go n.sendHave(a, h)
	}
}

// size is the bytes a program of the have takes in a store.
func (h Have) size() int64 { return int64(h.ManifestLength) + int64(h.PackLength) }

// onHave records who announced a program and puts the have in the have
// queue. It never starts a download (#42).
func (n *Node) onHave(h Have, from net.Addr) {
	n.mu.Lock()
	n.stats.HavesReceived++
	known := false
	for _, a := range n.announcers[h.Address] {
		known = known || a.String() == from.String()
	}
	if !known {
		n.announcers[h.Address] = append(n.announcers[h.Address], from)
	}
	n.mu.Unlock()
	n.haves.push(QueuedHave{Have: h, From: from.String(), Received: time.Now()})
}

// Update reads every have in the queue, applies the node type's policy to
// each, and returns them in arrival order so the program sees every one. A
// medium node requests a program that is pinned or fits its budget, a super
// node one that fits its capacity. A requested program is downloaded in the
// background, held and relayed.
func (n *Node) Update() []QueuedHave {
	read := n.haves.Read()
	for _, q := range read {
		n.applyPolicy(q)
	}
	return read
}

// RunUpdates calls Update whenever haves wait, and seen with each have
// read, until the node is closed.
func (n *Node) RunUpdates(seen func(QueuedHave)) {
	for {
		select {
		case <-n.closed:
			return
		case <-n.haves.Ready():
		}
		for _, q := range n.Update() {
			if seen != nil {
				seen(q)
			}
		}
	}
}

func (n *Node) applyPolicy(q QueuedHave) {
	h := q.Have
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stats.HavesRead++
	if n.requested[h.Address] || n.catalog.Holds(h.Address) {
		n.stats.HavesDuplicate++
		return
	}
	fits := n.settings.Budget == 0 || n.stats.StoredBytes+n.reservedBytes+h.size() <= n.settings.Budget
	switch n.settings.Type {
	case TypeMedium:
		if !n.pins.pinned(h.Address, n.logf) && (n.settings.Budget == 0 || !fits) {
			n.logf("have %s from %s read, not pinned and no room in the budget for %d bytes, not requested", h.Address, q.From, h.size())
			return
		}
	case TypeSuper:
		if !fits {
			n.stats.HavesRefusedFull++
			n.logf("storage full, not requesting %s of %d bytes, %d of %d bytes held", h.Address, h.size(), n.stats.StoredBytes, n.settings.Budget)
			return
		}
	}
	n.requested[h.Address] = true
	n.reservedBytes += h.size()
	n.stats.Requests++
	n.logf("have %s from %s read, requesting %d manifest bytes, %d packing bytes", h.Address, q.From, h.ManifestLength, h.PackLength)
	go n.download(h)
}

// download fetches a requested program from the nodes that announced it,
// verifies it, keeps it and relays it. A program from an untrusted signer,
// or one whose lengths differ from the have, is dropped for good. A full
// disk drops the partial program. Any other failure lets a later have
// request it again.
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
	delete(n.requested, h.Address)
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
	delete(n.requested, addr)
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

// Fetch requests the program at addr on demand and returns it verified:
// from the store when it is cached there, or else downloaded from the nodes
// given and then the ones that announced it. The node holds and relays
// it, pinned or not and over its budget or not.
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
	n.mu.Unlock()
	if len(sources) == 0 {
		return Held{}, fmt.Errorf("no node is known to hold %s", addr)
	}
	var last error
	for attempt := range max(DownloadAttempts, len(sources)) {
		from := sources[attempt%len(sources)]
		p, encoded, err := n.fetchFrom(Have{Address: addr}, from)
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

// keepFetched holds a fetched program.
func (n *Node) keepFetched(p Held, encoded []byte) error {
	if n.Holds(p.Address) {
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

// hold adds a verified program to the catalog, tells OnHeld and relays it.
func (n *Node) hold(p Held, encoded []byte) error {
	n.mu.Lock()
	already := n.catalog.Holds(p.Address)
	_, err := n.catalog.AddVerified(encoded, packFromFiles(p.Manifest, p.Files))
	n.requested[p.Address] = true
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
	n.relay(p.Address)
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

// Publish signs files as a new program with secret, keeps it, and
// announces it to the subscribers. The signer must be one of the node's
// trusted keys.
func (n *Node) Publish(files map[string][]byte, secret cipher.SecKey, date time.Time) (programblocks.Hash, error) {
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

// SetPeers sets the node's peer list, a copy of l. Get peers answers from
// it and GetPeers learns into it.
func (n *Node) SetPeers(l *PeerList) {
	n.mu.Lock()
	n.peers = l.Clone()
	n.mu.Unlock()
}

// Peers returns a copy of the node's peer list, learned peers included.
func (n *Node) Peers() *PeerList {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.peers == nil {
		return &PeerList{}
	}
	return n.peers.Clone()
}

// Store is the store the node keeps its programs in.
func (n *Node) Store() *programstore.Store { return n.store }
