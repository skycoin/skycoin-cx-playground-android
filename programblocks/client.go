package programblocks

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"os"
	"sort"
	"syscall"
	"time"

	"github.com/skycoin/skycoin/src/cipher"
)

// Client defaults, see "Loss, retries and order" in
// docs/ProgramDistribution.md.
const (
	DefaultWindow       = 16
	DefaultFirstTimeout = 250 * time.Millisecond
	DefaultMaxTimeout   = 2 * time.Second
	DefaultMaxTries     = 8
	MaxBadRangeReplies  = 3
	maxBlockRounds      = 4
)

// Forms is the client config flag that names the request forms (D37).
type Forms int

// FormsFull uses the full forms (0x01 to 0x04) and whole units, FormsShort
// the short forms (0x05 to 0x08) and parts of units. A client never
// switches between them by itself.
const (
	FormsFull Forms = iota
	FormsShort
)

// String returns the flag value of f.
func (f Forms) String() string {
	if f == FormsShort {
		return "short"
	}
	return "full"
}

// ParseForms reads the forms flag value full or short.
func ParseForms(s string) (Forms, error) {
	switch s {
	case "full":
		return FormsFull, nil
	case "short":
		return FormsShort, nil
	}
	return FormsFull, fmt.Errorf("forms %q, want full or short", s)
}

// ErrPacketTooSmall marks an operation whose request or reply does not fit
// the configured packet size and forms.
var ErrPacketTooSmall = errors.New("error 6, packet too small")

// PacketTooSmall returns error 6 for an operation that does not fit the
// configured packet size and forms, naming the flags that change it.
func PacketTooSmall(packetSize int, forms Forms, need int, format string, a ...any) error {
	hint := fmt.Sprintf("raise the packet flag to %d or more", need)
	if forms == FormsFull {
		hint += " or set the forms flag to short"
	}
	return fmt.Errorf("%w: %s at packet %d in the %s forms, %s", ErrPacketTooSmall, fmt.Sprintf(format, a...), packetSize, forms, hint)
}

func (c *Client) tooSmall(need int, format string, a ...any) error {
	return PacketTooSmall(c.PacketSize, c.Forms, need, format, a...)
}

// Client fetches and verifies programs from a server over a connected UDP
// socket.
type Client struct {
	Conn          net.Conn
	PacketSize    int
	Forms         Forms
	PrefixLength  int // longest prefix the short forms send, DefaultPrefixLength when 0
	Window        int
	FirstTimeout  time.Duration
	MaxTimeout    time.Duration
	MaxTries      int
	PinnedSigners []cipher.PubKey // when set, other signers are refused after the first packet
	Stats         ClientStats
	nextRequestID uint32
}

// ClientStats counts the packets of a client.
type ClientStats struct {
	RequestsSent     int
	Retransmissions  int
	RepliesAccepted  int
	RepliesDiscarded int
	BytesSent        int
	BytesReceived    int
	LargestReply     int
	RangesRejected   int // get blocks replies that failed a block hash
}

// NewClient returns a client with the default settings. It accepts any
// manifest whose signature is valid (D27), or with pinned keys only those
// signed by one of them.
func NewClient(conn net.Conn, pinned ...cipher.PubKey) *Client {
	return &Client{Conn: conn, PacketSize: DefaultPacketSize, PrefixLength: DefaultPrefixLength, Window: DefaultWindow, FirstTimeout: DefaultFirstTimeout,
		MaxTimeout: DefaultMaxTimeout, MaxTries: DefaultMaxTries, PinnedSigners: pinned, nextRequestID: rand.Uint32()}
}

// ShortForms reports whether the client uses the short forms.
func (c *Client) ShortForms() bool {
	return c.Forms == FormsShort
}

// prefixFitting returns the longest prefix of at most the client's prefix
// length that fits in room bytes, or 0 when MinPrefixLength does not fit.
func (c *Client) prefixFitting(room int) int {
	setting := c.PrefixLength
	if setting == 0 {
		setting = DefaultPrefixLength
	}
	n := min(setting, len(Hash{}), room)
	if n < MinPrefixLength {
		return 0
	}
	return n
}

type requestInFlight struct {
	request Request
	sent    time.Time
	timeout time.Duration
	tries   int
}

// replyHandler handles the reply to request and returns any requests to
// send next.
type replyHandler func(request Request, reply Reply) ([]Request, error)

func replyMatches(req Request, rep Reply) bool {
	switch req.Operation {
	case OperationListManifests:
		return true
	case OperationListManifestsPrefix:
		return rep.PrefixLength == req.PrefixLength
	case OperationGetBlockPart:
		return rep.BlockPrefixLength == req.BlockPrefixLength && rep.BlockHash.HasPrefix(req.BlockHash, req.BlockPrefixLength)
	case OperationGetManifestByPrefix, OperationGetBlockListBytes:
		return rep.PrefixLength == req.PrefixLength && rep.Address.HasPrefix(req.Address, req.PrefixLength)
	}
	return rep.Address == req.Address
}

// exchange sends requests, at most Window at a time, resends each one whose
// reply is late, and hands every reply to handle.
func (c *Client) exchange(requests []Request, handle replyHandler) error {
	if err := CheckPacketSize(c.PacketSize); err != nil {
		return err
	}
	queue := append([]Request(nil), requests...)
	inFlight := map[uint32]*requestInFlight{}
	buf := make([]byte, 65536)
	send := func(f *requestInFlight) error {
		f.sent = time.Now()
		f.tries++
		c.Stats.RequestsSent++
		b := EncodeRequest(f.request)
		c.Stats.BytesSent += len(b)
		_, err := c.Conn.Write(b)
		if errors.Is(err, syscall.ECONNREFUSED) {
			err = nil // the server is not up yet; the retry covers it
		}
		return err
	}
	for len(queue) > 0 || len(inFlight) > 0 {
		for len(queue) > 0 && len(inFlight) < c.Window {
			c.nextRequestID++
			r := queue[0]
			queue = queue[1:]
			if n := RequestLength(r); n > c.PacketSize {
				return c.tooSmall(n, "operation %#x request of %d bytes", byte(r.Operation), n)
			}
			r.RequestID = c.nextRequestID
			f := &requestInFlight{request: r, timeout: c.FirstTimeout}
			inFlight[r.RequestID] = f
			if err := send(f); err != nil {
				return err
			}
		}
		var deadline time.Time
		for _, f := range inFlight {
			if d := f.sent.Add(f.timeout); deadline.IsZero() || d.Before(deadline) {
				deadline = d
			}
		}
		c.Conn.SetReadDeadline(deadline)
		n, err := c.Conn.Read(buf)
		if err != nil {
			if !errors.Is(err, os.ErrDeadlineExceeded) && !errors.Is(err, syscall.ECONNREFUSED) {
				return err
			}
			now := time.Now()
			for _, f := range inFlight {
				if now.Before(f.sent.Add(f.timeout)) {
					continue
				}
				if f.tries >= c.MaxTries {
					return fmt.Errorf("no reply to operation %#x after %d tries", byte(f.request.Operation), f.tries)
				}
				f.timeout = min(2*f.timeout, c.MaxTimeout)
				c.Stats.Retransmissions++
				if err := send(f); err != nil {
					return err
				}
			}
			continue
		}
		reply, err := DecodeReply(buf[:n])
		f := inFlight[reply.RequestID]
		if err != nil || f == nil {
			c.Stats.RepliesDiscarded++
			continue
		}
		delete(inFlight, reply.RequestID)
		c.Stats.RepliesAccepted++
		c.Stats.BytesReceived += n
		c.Stats.LargestReply = max(c.Stats.LargestReply, n)
		if reply.Operation == OperationError {
			text := reply.ErrorText
			if reply.ErrorCode == ErrorCodePacketTooSmall && c.Forms == FormsFull {
				text += ", set the forms flag to short"
			}
			return &ReplyError{Code: reply.ErrorCode, Text: text}
		}
		if reply.Operation != f.request.Operation|ReplyFlag || !replyMatches(f.request, reply) {
			return fmt.Errorf("%w: reply does not match its request", ErrPacket)
		}
		next, err := handle(f.request, reply)
		if err != nil {
			return err
		}
		queue = append(queue, next...)
	}
	return nil
}

// ListManifests returns the address and length of the server's manifest
// list with the full form, list manifests (0x01).
func (c *Client) ListManifests() (Hash, uint32, error) {
	var addr Hash
	var length uint32
	err := c.exchange([]Request{{Operation: OperationListManifests}}, func(_ Request, r Reply) ([]Request, error) {
		addr, length = r.Address, r.Total
		return nil, nil
	})
	return addr, length, err
}

// ListManifestsPrefix returns a prefix of the address of the server's
// manifest list, its length in bytes and the manifest list's length, with
// the short form, list manifests prefix (0x05).
func (c *Client) ListManifestsPrefix() (Hash, int, uint32, error) {
	var addr Hash
	var length uint32
	n := c.prefixFitting(c.PacketSize - listPrefixReplyFixed)
	if n == 0 {
		return addr, 0, 0, c.tooSmall(listPrefixReplyFixed+MinPrefixLength, "list manifests prefix reply")
	}
	err := c.exchange([]Request{{Operation: OperationListManifestsPrefix, PrefixLength: n}}, func(_ Request, r Reply) ([]Request, error) {
		addr, length = r.Address, r.Total
		return nil, nil
	})
	return addr, n, length, err
}

// FetchManifestList fetches and verifies the server's manifest list and
// returns it, its bytes and its address, with the forms the client uses.
func (c *Client) FetchManifestList() (*Manifest, []byte, Hash, error) {
	if c.ShortForms() {
		prefix, n, _, err := c.ListManifestsPrefix()
		if err != nil {
			return nil, nil, Hash{}, err
		}
		return c.fetchManifest(prefix, n)
	}
	addr, _, err := c.ListManifests()
	if err != nil {
		return nil, nil, addr, err
	}
	return c.fetchManifest(addr, len(addr))
}

// pinned reports whether a manifest signed by key is accepted.
func (c *Client) pinned(key cipher.PubKey) bool {
	return len(c.PinnedSigners) == 0 || KeyListed(c.PinnedSigners, key)
}

// FetchManifest fetches the manifest at addr in reply-sized pieces. It
// checks the address, signature and signer as soon as the envelope header
// is in, and the hash of the data and the whole manifest at the end.
func (c *Client) FetchManifest(addr Hash) (*Manifest, []byte, error) {
	m, b, _, err := c.fetchManifest(addr, len(addr))
	return m, b, err
}

// FetchManifestByPrefix fetches the manifest whose address starts with the
// first n bytes of prefix and returns it with its full address. Only get
// manifest by prefix (0x06) takes a prefix, so it needs the short forms.
func (c *Client) FetchManifestByPrefix(prefix Hash, n int) (*Manifest, []byte, Hash, error) {
	if c.Forms != FormsShort {
		return nil, nil, Hash{}, errors.New("a manifest address prefix needs the short forms, set the forms flag to short or give the whole address")
	}
	return c.fetchManifest(prefix, n)
}

// fetchManifest fetches the manifest whose address starts with the first
// known bytes of addr and returns it with its full address.
func (c *Client) fetchManifest(addr Hash, known int) (*Manifest, []byte, Hash, error) {
	var ask func(off, n int) Request
	var piece int
	if c.Forms == FormsShort {
		n := min(known, c.prefixFitting(c.PacketSize-byPrefixReplyFixed-1))
		if n < MinPrefixLength {
			return nil, nil, Hash{}, c.tooSmall(byPrefixReplyFixed+MinPrefixLength+1, "get manifest by prefix reply")
		}
		piece = MaxByPrefixPayload(c.PacketSize, n)
		ask = func(off, length int) Request {
			return Request{Operation: OperationGetManifestByPrefix, Address: addr, PrefixLength: n, Offset: uint64(off), Length: uint16(length)}
		}
	} else {
		piece = MaxManifestPayload(c.PacketSize)
		if piece < 1 {
			return nil, nil, Hash{}, c.tooSmall(manifestReplyFixed+1, "get manifest reply")
		}
		ask = func(off, length int) Request {
			return Request{Operation: OperationGetManifest, Address: addr, Offset: uint64(off), Length: uint16(length)}
		}
	}
	piece = min(piece, 0xFFFF)
	var buf []byte
	var have []bool
	var header *Header
	var full Hash
	err := c.exchange([]Request{ask(0, piece)}, func(req Request, r Reply) ([]Request, error) {
		var next []Request
		if buf == nil {
			if int(r.Total) < MinEnvelopeLength || r.Total > MaxManifestLength {
				return nil, fmt.Errorf("%w: manifest length %d", ErrMalformed, r.Total)
			}
			buf, have = make([]byte, r.Total), make([]bool, r.Total)
			for off := piece; off < int(r.Total); off += piece {
				next = append(next, ask(off, min(piece, int(r.Total)-off)))
			}
		}
		if int(r.Total) != len(buf) || r.Offset != req.Offset || r.Offset+uint64(len(r.Payload)) > uint64(len(buf)) || len(r.Payload) == 0 {
			return nil, fmt.Errorf("%w: get manifest reply out of range", ErrPacket)
		}
		copy(buf[r.Offset:], r.Payload)
		for i := range r.Payload {
			have[int(r.Offset)+i] = true
		}
		if off, end := int(r.Offset)+len(r.Payload), min(int(req.Offset)+int(req.Length), len(buf)); off < end {
			next = append(next, ask(off, end-off))
		}
		if header == nil {
			n, err := HeaderLength(buf)
			if err == nil && allTrue(have[:min(n, len(have))]) {
				h, got, err := ParseHeader(buf)
				if err != nil {
					return nil, err
				}
				if !got.HasPrefix(addr, known) {
					return nil, fmt.Errorf("%w: got %s, want %x", ErrAddress, got, addr[:known])
				}
				if !c.pinned(h.SignerPublicKey) {
					return nil, fmt.Errorf("%w: %s", ErrNotPinned, h.SignerPublicKey.Hex())
				}
				header, full = &h, got
			}
		}
		return next, nil
	})
	if err != nil {
		return nil, nil, full, err
	}
	m, _, err := Decode(buf, full)
	return m, buf, full, err
}

// FetchBlockEntries reads count block list entries from block ID first of
// the manifest at addr, with get block list (0x03) in the full forms and
// get block list bytes (0x07) in the short forms. The entries are not
// verified: a caller checks them against a file's Merkle hash.
func (c *Client) FetchBlockEntries(addr Hash, first, count int) ([]BlockEntry, error) {
	if first < 1 || count < 1 {
		return nil, fmt.Errorf("block entries %d+%d", first, count)
	}
	if !c.ShortForms() {
		per := MaxBlockListEntries(c.PacketSize)
		if per < 1 {
			return nil, c.tooSmall(blockListReplyFixed+BlockEntryLength, "a %d-byte block entry in a get block list reply", BlockEntryLength)
		}
		out := make([]BlockEntry, count)
		ask := func(id, n int) Request {
			return Request{Operation: OperationGetBlockList, Address: addr, Offset: uint64(id), Length: uint16(n)}
		}
		var requests []Request
		for id := first; id < first+count; id += per {
			requests = append(requests, ask(id, min(per, first+count-id)))
		}
		err := c.exchange(requests, func(req Request, r Reply) ([]Request, error) {
			if r.Offset != req.Offset || r.Length == 0 || r.Length > req.Length {
				return nil, fmt.Errorf("%w: get block list reply out of range", ErrPacket)
			}
			for k := range int(r.Length) {
				out[int(r.Offset)-first+k] = DecodeBlockEntry(r.Payload[k*BlockEntryLength:])
			}
			if r.Length < req.Length {
				return []Request{ask(int(r.Offset)+int(r.Length), int(req.Length-r.Length))}, nil
			}
			return nil, nil
		})
		return out, err
	}
	n := c.prefixFitting(c.PacketSize - byPrefixReplyFixed - 1)
	if n == 0 {
		return nil, c.tooSmall(byPrefixReplyFixed+MinPrefixLength+1, "get block list bytes reply")
	}
	piece := min(MaxByPrefixPayload(c.PacketSize, n), 0xFFFF)
	start, length := (first-1)*BlockEntryLength, count*BlockEntryLength
	buf := make([]byte, length)
	ask := func(off, l int) Request {
		return Request{Operation: OperationGetBlockListBytes, Address: addr, PrefixLength: n, Offset: uint64(start + off), Length: uint16(l)}
	}
	var requests []Request
	for off := 0; off < length; off += piece {
		requests = append(requests, ask(off, min(piece, length-off)))
	}
	err := c.exchange(requests, func(req Request, r Reply) ([]Request, error) {
		got := int(r.Offset) - start
		if r.Offset != req.Offset || len(r.Payload) == 0 || len(r.Payload) > int(req.Length) {
			return nil, fmt.Errorf("%w: get block list bytes reply out of range", ErrPacket)
		}
		copy(buf[got:], r.Payload)
		if len(r.Payload) < int(req.Length) {
			return []Request{ask(got+len(r.Payload), int(req.Length)-len(r.Payload))}, nil
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]BlockEntry, count)
	for k := range out {
		out[k] = DecodeBlockEntry(buf[k*BlockEntryLength:])
		if out[k].BlockID != uint32(first+k) {
			return nil, fmt.Errorf("%w: block entry %d has block ID %d", ErrMalformed, first+k, out[k].BlockID)
		}
	}
	return out, nil
}

func allTrue(b []bool) bool {
	for _, v := range b {
		if !v {
			return false
		}
	}
	return true
}

// FetchFiles fetches the linear packing of m, the manifest at addr,
// verifies every block and file and returns the files.
func (c *Client) FetchFiles(addr Hash, m *Manifest) (map[string][]byte, error) {
	a := NewAssembly(m)
	if err := c.FetchInto(addr, a); err != nil {
		return nil, err
	}
	return a.Files()
}

// FetchInto fetches the blocks a, the assembly of the manifest at addr,
// still misses, and verifies each one. In the full forms it asks get blocks
// (0x04) for as many whole blocks as fit, two per packet at 1200 bytes, and
// fails with error 6 when a reply cannot hold a whole block. In the short
// forms it asks get block part (0x08) for parts of blocks. A store resumes a
// stopped download by giving an assembly that holds the blocks it kept.
func (c *Client) FetchInto(addr Hash, a *Assembly) error {
	if c.Forms == FormsFull {
		room := min(MaxBlocksPayload(c.PacketSize), 0xFFFF)
		if room < a.largestBlock() {
			return c.tooSmall(blocksReplyFixed+a.largestBlock(), "a %d-byte block in a get blocks reply", a.largestBlock())
		}
		return c.fetchWholeBlocks(addr, a, room)
	}
	blockPrefix := c.prefixFitting(c.PacketSize - blockPartReplyFixed - 1)
	if blockPrefix == 0 {
		return c.tooSmall(blockPartReplyFixed+MinPrefixLength+1, "get block part reply")
	}
	manifestPrefix := min(blockPrefix, c.PacketSize-blockPartRequestFixed-blockPrefix)
	if manifestPrefix < MinPrefixLength {
		manifestPrefix = 0
	}
	room := min(MaxBlockPartPayload(c.PacketSize, blockPrefix), 0xFFFF)
	m := a.manifest
	badReplies := map[int]int{}
	for round := 0; !a.Complete(); round++ {
		if round == maxBlockRounds {
			return errors.New("blocks still missing after the retries")
		}
		// Blocks with the same hash share one request, so each request
		// lists every block index it fills.
		targets := map[Request][]int{}
		var requests []Request
		add := func(i int, off, n uint64) {
			r := Request{Operation: OperationGetBlockPart, Address: addr, PrefixLength: manifestPrefix,
				BlockHash: m.Blocks[i].BlockHash, BlockPrefixLength: blockPrefix, Offset: off, Length: uint16(n)}
			if _, ok := targets[r]; !ok {
				requests = append(requests, r)
			}
			targets[r] = append(targets[r], i)
		}
		for _, p := range a.MissingParts(room) {
			add(p.Index, p.Offset, p.Length)
		}
		err := c.exchange(requests, func(req Request, r Reply) ([]Request, error) {
			req.RequestID = 0
			if r.Offset != req.Offset || len(r.Payload) == 0 || len(r.Payload) > int(req.Length) {
				return nil, fmt.Errorf("%w: block part reply at %d+%d, asked %d+%d", ErrPacket, r.Offset, len(r.Payload), req.Offset, req.Length)
			}
			var next []Request
			for _, i := range targets[req] {
				if r.Total != m.Packing[i].BlockLength {
					return nil, fmt.Errorf("%w: block %d of %d bytes, the manifest says %d", ErrPacket, i+1, r.Total, m.Packing[i].BlockLength)
				}
				if _, err := a.AcceptPart(i, req.Offset, r.Payload); err != nil {
					if !errors.Is(err, ErrBlockHash) {
						return nil, err
					}
					badReplies[i]++
					c.Stats.RangesRejected++
					if badReplies[i] >= MaxBadRangeReplies {
						return nil, err
					}
				}
			}
			if rest := uint64(req.Length) - uint64(len(r.Payload)); rest > 0 {
				before := len(requests)
				for _, i := range targets[req] {
					add(i, req.Offset+uint64(len(r.Payload)), rest)
				}
				next = append(next, requests[before:]...)
			}
			return next, nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// fetchWholeBlocks fetches the missing blocks of a with get blocks, as
// many whole blocks per request as fit in room bytes. A reply that holds
// no whole block, from a server with a smaller packet size, is error 6.
func (c *Client) fetchWholeBlocks(addr Hash, a *Assembly, room int) error {
	badReplies := map[uint64]int{}
	for round := 0; !a.Complete(); round++ {
		if round == maxBlockRounds {
			return errors.New("blocks still missing after the retries")
		}
		var requests []Request
		for _, r := range a.MissingRanges(room) {
			requests = append(requests, Request{Operation: OperationGetBlocks, Address: addr, Offset: r[0], Length: uint16(r[1])})
		}
		err := c.exchange(requests, func(req Request, r Reply) ([]Request, error) {
			if r.Offset != req.Offset {
				return nil, fmt.Errorf("%w: get blocks reply at %d, asked %d", ErrPacket, r.Offset, req.Offset)
			}
			if _, err := a.Accept(r.Offset, r.Payload); err != nil {
				if !errors.Is(err, ErrBlockHash) {
					return nil, err
				}
				badReplies[req.Offset]++
				c.Stats.RangesRejected++
				if badReplies[req.Offset] >= MaxBadRangeReplies {
					return nil, err
				}
				return []Request{req}, nil
			}
			end, want := r.Offset+uint64(len(r.Payload)), req.Offset+uint64(req.Length)
			if end >= want {
				return nil, nil
			}
			// A shorter reply holds whole blocks: ask again from the first
			// block it did not hold whole.
			for _, p := range a.manifest.Packing {
				if p.PackOffset >= req.Offset && p.PackOffset+uint64(p.BlockLength) > end {
					if p.PackOffset == req.Offset {
						return nil, fmt.Errorf("%w: the server's get blocks reply held %d bytes, no whole block, set the forms flag to short", ErrPacketTooSmall, len(r.Payload))
					}
					return []Request{{Operation: OperationGetBlocks, Address: addr, Offset: p.PackOffset, Length: uint16(want - p.PackOffset)}}, nil
				}
			}
			return nil, nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ProgramNames returns the program names of the manifest list m.
func ProgramNames(m *Manifest) []string {
	var names []string
	for _, f := range m.Files {
		names = append(names, f.Path)
	}
	sort.Strings(names)
	return names
}

// FetchedProgram is a program fetched and verified by FetchProgram.
type FetchedProgram struct {
	Address  Hash
	Manifest *Manifest
	Files    map[string][]byte
}

// FetchProgram lists the server's programs, fetches the manifest list and
// the program called name, and returns its verified files. The program
// manifest comes inside the verified manifest list, so it is not fetched
// again, but its signature and signer are checked on their own.
func (c *Client) FetchProgram(name string) (*FetchedProgram, error) {
	list, _, listAddr, err := c.FetchManifestList()
	if err != nil {
		return nil, fmt.Errorf("manifest list: %w", err)
	}
	manifests, err := c.FetchFiles(listAddr, list)
	if err != nil {
		return nil, fmt.Errorf("manifest list: %w", err)
	}
	b, ok := manifests[name]
	if !ok {
		return nil, fmt.Errorf("the server has no program %q, it has %q", name, ProgramNames(list))
	}
	m, addr, err := Decode(b, Hash{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if !c.pinned(m.SignerPublicKey) {
		return nil, fmt.Errorf("%s: %w: %s", name, ErrNotPinned, m.SignerPublicKey.Hex())
	}
	files, err := c.FetchFiles(addr, m)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &FetchedProgram{Address: addr, Manifest: m, Files: files}, nil
}
