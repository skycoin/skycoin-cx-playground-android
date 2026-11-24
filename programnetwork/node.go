package programnetwork

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
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

	conn    net.PacketConn
	store   *programstore.Store
	trusted []cipher.PubKey

	mu          sync.Mutex
	catalog     *programblocks.Catalog
	seen        map[programblocks.Hash]bool
	announcers  map[programblocks.Hash][]net.Addr
	announced   map[programblocks.Hash]bool
	subscribers map[string]subscriber
	pending     map[uint32]chan []byte
	nextID      uint32
	stats       Stats
	peers       *PeerList

	// storeMu serializes the writes to the store and its index.
	storeMu   sync.Mutex
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
	HavesReceived       int // have packets accepted
	HavesDuplicate      int // haves for a program already held or on its way
	HavesSent           int // haves a subscriber acknowledged
	HavesUnacknowledged int // haves given up after the retries
	Downloads           int // programs downloaded and verified
	DownloadFailures    int // programs given up after DownloadAttempts
	Subscribers         int // live subscribers
}

// NewNode returns a node that answers on conn and keeps programs in store.
// It downloads only programs signed by one of trusted, and starts out
// holding every cached program of the store.
func NewNode(conn net.PacketConn, store *programstore.Store, trusted []cipher.PubKey) (*Node, error) {
	n := &Node{
		PacketSize: programblocks.DefaultPacketSize, Lease: DefaultLease,
		FirstTimeout: programblocks.DefaultFirstTimeout, MaxTimeout: programblocks.DefaultMaxTimeout, MaxTries: programblocks.DefaultMaxTries,
		conn: conn, store: store, trusted: trusted,
		catalog: programblocks.EmptyCatalog(), seen: map[programblocks.Hash]bool{}, announcers: map[programblocks.Hash][]net.Addr{},
		announced: map[programblocks.Hash]bool{}, subscribers: map[string]subscriber{}, pending: map[uint32]chan []byte{},
		nextID: rand.Uint32(), closed: make(chan struct{}),
	}
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
		if _, err := n.catalog.AddVerified(m.Encode(), packFromFiles(m, files)); err != nil {
			return nil, fmt.Errorf("%s: %w", r.Name, err)
		}
		n.seen[addr] = true
	}
	return n, nil
}

// Addr is the address the node answers on.
func (n *Node) Addr() net.Addr { return n.conn.LocalAddr() }

// Stats returns the node's counters.
func (n *Node) Stats() Stats {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.stats
	s.Subscribers = len(n.subscribers)
	return s
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

// Close stops the node. Run returns.
func (n *Node) Close() error {
	n.closeOnce.Do(func() { close(n.closed) })
	return n.conn.Close()
}

// Run answers packets until the node is closed.
func (n *Node) Run() error {
	if err := programblocks.CheckPacketSize(n.PacketSize); err != nil {
		return err
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
		var shared []PeerEntry
		n.mu.Lock()
		peers := n.peers
		n.mu.Unlock()
		if peers != nil {
			shared = peers.SharedWith(from.String())
		}
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
// every program held (D34).
func (n *Node) subscribe(from net.Addr, leaseSeconds uint32) uint32 {
	lease := min(time.Duration(leaseSeconds)*time.Second, n.Lease)
	key := from.String()
	n.mu.Lock()
	_, known := n.subscribers[key]
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
func (n *Node) Subscribe(address string) error {
	to, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	go func() {
		var lease uint32
		for {
			l, err := n.subscribeOnce(to)
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

func (n *Node) subscribeOnce(to net.Addr) (uint32, error) {
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
	for {
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
		all = append(all, r.Entries...)
		if len(all) >= int(r.Total) || len(r.Entries) == 0 {
			return all, nil
		}
	}
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
	for key, s := range n.subscribers {
		if now.After(s.expires) {
			delete(n.subscribers, key)
			continue
		}
		to = append(to, s.addr)
	}
	n.mu.Unlock()
	n.logf("announce %s to %d subscribers", addr, len(to))
	for _, a := range to {
		go n.sendHave(a, h)
	}
}

// onHave starts the download of a program the node has not seen.
func (n *Node) onHave(h Have, from net.Addr) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stats.HavesReceived++
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
	n.seen[h.Address] = true
	n.logf("have %s from %s, %d manifest bytes, %d packing bytes", h.Address, from, h.ManifestLength, h.PackLength)
	go n.download(h)
}

// download fetches a program from the nodes that announced it, verifies
// it, keeps it and announces it in turn. A program from an untrusted
// signer, or one whose lengths differ from the have, is dropped for good.
// Any other failure lets a later have start over.
func (n *Node) download(h Have) {
	for attempt := range DownloadAttempts {
		n.mu.Lock()
		list := n.announcers[h.Address]
		from := list[attempt%len(list)]
		n.mu.Unlock()
		err := n.fetchFrom(h, from)
		if err == nil {
			return
		}
		n.logf("download %s from %s: %v", h.Address, from, err)
		if errors.Is(err, programblocks.ErrUntrusted) || errors.Is(err, errHaveMismatch) {
			return
		}
	}
	n.mu.Lock()
	n.stats.DownloadFailures++
	delete(n.seen, h.Address)
	n.mu.Unlock()
}

var errHaveMismatch = errors.New("manifest lengths differ from the have")

func (n *Node) fetchFrom(h Have, from net.Addr) error {
	start := time.Now()
	conn, err := net.Dial("udp", from.String())
	if err != nil {
		return err
	}
	defer conn.Close()
	c := programblocks.NewClient(conn, n.trusted...)
	c.PacketSize, c.FirstTimeout, c.MaxTimeout, c.MaxTries = n.PacketSize, n.FirstTimeout, n.MaxTimeout, n.MaxTries
	m, encoded, err := c.FetchManifest(h.Address)
	if err != nil {
		return err
	}
	if uint32(len(encoded)) != h.ManifestLength || m.PackLength != h.PackLength {
		return fmt.Errorf("%w: %d and %d bytes, have says %d and %d", errHaveMismatch, len(encoded), m.PackLength, h.ManifestLength, h.PackLength)
	}
	n.storeMu.Lock()
	defer n.storeMu.Unlock()
	if _, _, err := n.store.SaveManifest(encoded, n.trusted); err != nil {
		return err
	}
	if err := n.setRow(h.Address, programstore.StateRemote, m.PackLength, h.Address.String()[:16]); err != nil {
		return err
	}
	files, err := n.store.Download(c, h.Address)
	if err != nil {
		return err
	}
	name := programName(files, h.Address)
	if err := n.setRow(h.Address, programstore.StateCached, m.PackLength, name); err != nil {
		return err
	}
	n.mu.Lock()
	n.stats.Downloads++
	n.mu.Unlock()
	return n.hold(Held{Address: h.Address, Manifest: m, Files: files, Name: name, From: from.String(), Elapsed: time.Since(start)}, encoded)
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
	_, err := n.catalog.AddVerified(encoded, packFromFiles(p.Manifest, p.Files))
	n.seen[p.Address] = true
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

// SetPeers sets the peer list get peers answers from. The node does not
// change the list. With no list it shares no peer.
func (n *Node) SetPeers(l *PeerList) {
	n.mu.Lock()
	n.peers = l
	n.mu.Unlock()
}

// Store is the store the node keeps its programs in.
func (n *Node) Store() *programstore.Store { return n.store }
