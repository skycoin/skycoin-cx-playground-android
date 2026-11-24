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

// Forms selects the request forms a client uses.
type Forms int

// FormsAuto uses the full forms (0x01 to 0x04) at MinFullFormPacketSize
// and up and the short forms (0x05 to 0x08) below it (D37).
const (
	FormsAuto Forms = iota
	FormsFull
	FormsShort
)

// ErrPacketTooSmall marks an operation whose request or smallest reply does
// not fit the packet size.
var ErrPacketTooSmall = errors.New("packet size too small for the operation")

// Client fetches and verifies programs from a server over a connected UDP
// socket.
type Client struct {
	Conn           net.Conn
	PacketSize     int
	Forms          Forms
	PrefixLength   int // longest prefix the short forms send, DefaultPrefixLength when 0
	Window         int
	FirstTimeout   time.Duration
	MaxTimeout     time.Duration
	MaxTries       int
	TrustedSigners []cipher.PubKey
	Stats          ClientStats
	nextRequestID  uint32
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

// NewClient returns a client with the default settings that accepts
// manifests signed by any of trusted.
func NewClient(conn net.Conn, trusted ...cipher.PubKey) *Client {
	return &Client{Conn: conn, PacketSize: DefaultPacketSize, PrefixLength: DefaultPrefixLength, Window: DefaultWindow, FirstTimeout: DefaultFirstTimeout,
		MaxTimeout: DefaultMaxTimeout, MaxTries: DefaultMaxTries, TrustedSigners: trusted, nextRequestID: rand.Uint32()}
}

// ShortForms reports whether the client uses the short forms.
func (c *Client) ShortForms() bool {
	switch c.Forms {
	case FormsFull:
		return false
	case FormsShort:
		return true
	}
	return c.PacketSize < MinFullFormPacketSize
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
				return fmt.Errorf("%w: operation %#x request of %d bytes over %d", ErrPacketTooSmall, byte(r.Operation), n, c.PacketSize)
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
			return &ReplyError{Code: reply.ErrorCode, Text: reply.ErrorText}
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
		return addr, 0, 0, fmt.Errorf("%w: list manifests prefix reply needs %d bytes", ErrPacketTooSmall, listPrefixReplyFixed+MinPrefixLength)
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

func (c *Client) trusted(key cipher.PubKey) bool {
	for _, k := range c.TrustedSigners {
		if k == key {
			return true
		}
	}
	return false
}

// FetchManifest fetches the manifest at addr in reply-sized pieces. It
// checks the address, signature and signer as soon as the header is in,
// each array as soon as it is complete, and the whole manifest at the end.
func (c *Client) FetchManifest(addr Hash) (*Manifest, []byte, error) {
	m, b, _, err := c.fetchManifest(addr, len(addr))
	return m, b, err
}

// fetchManifest fetches the manifest whose address starts with the first
// known bytes of addr and returns it with its full address.
func (c *Client) fetchManifest(addr Hash, known int) (*Manifest, []byte, Hash, error) {
	var ask func(off, n int) Request
	var piece int
	if c.ShortForms() || known < len(addr) {
		n := min(known, c.prefixFitting(c.PacketSize-byPrefixReplyFixed-1))
		if n < MinPrefixLength {
			return nil, nil, Hash{}, fmt.Errorf("%w: get manifest by prefix reply needs %d bytes", ErrPacketTooSmall, byPrefixReplyFixed+MinPrefixLength+1)
		}
		piece = MaxByPrefixPayload(c.PacketSize, n)
		ask = func(off, length int) Request {
			return Request{Operation: OperationGetManifestByPrefix, Address: addr, PrefixLength: n, Offset: uint64(off), Length: uint16(length)}
		}
	} else {
		piece = MaxManifestPayload(c.PacketSize)
		if piece < 1 {
			return nil, nil, Hash{}, fmt.Errorf("%w: get manifest reply needs %d bytes", ErrPacketTooSmall, manifestReplyFixed+1)
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
	verified := map[int]bool{}
	err := c.exchange([]Request{ask(0, piece)}, func(req Request, r Reply) ([]Request, error) {
		var next []Request
		if buf == nil {
			if r.Total < HeaderFixedLength || r.Total > MaxManifestLength {
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
				if !c.trusted(h.SignerPublicKey) {
					return nil, fmt.Errorf("%w: %s", ErrUntrusted, h.SignerPublicKey.Hex())
				}
				header, full = &h, got
			}
		}
		if header != nil {
			for i, e := range header.Arrays {
				end := uint64(e.Offset) + uint64(e.Length)
				if verified[i] || end > uint64(len(have)) || !allTrue(have[e.Offset:end]) {
					continue
				}
				if !ArrayVerified(buf, e) {
					return nil, fmt.Errorf("%w: kind %d", ErrArrayHash, e.Kind)
				}
				verified[i] = true
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
			return nil, fmt.Errorf("%w: a %d-byte block entry needs a %d-byte get block list reply", ErrPacketTooSmall, BlockEntryLength, blockListReplyFixed+BlockEntryLength)
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
		return nil, fmt.Errorf("%w: get block list bytes reply needs %d bytes", ErrPacketTooSmall, byPrefixReplyFixed+MinPrefixLength+1)
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
// still misses, and verifies each one. When a reply holds whole blocks it
// asks for as many as fit, two per packet at 1200 bytes. Below that it
// asks for parts of blocks, with get blocks (0x04) in the full forms and
// get block part (0x08) in the short forms. A store resumes a stopped
// download by giving an assembly that holds the blocks it kept.
func (c *Client) FetchInto(addr Hash, a *Assembly) error {
	short := c.ShortForms()
	var room, blockPrefix, manifestPrefix int
	if short {
		blockPrefix = c.prefixFitting(c.PacketSize - blockPartReplyFixed - 1)
		if blockPrefix == 0 {
			return fmt.Errorf("%w: get block part reply needs %d bytes", ErrPacketTooSmall, blockPartReplyFixed+MinPrefixLength+1)
		}
		if manifestPrefix = min(blockPrefix, c.PacketSize-blockPartRequestFixed-blockPrefix); manifestPrefix < MinPrefixLength {
			manifestPrefix = 0
		}
		room = MaxBlockPartPayload(c.PacketSize, blockPrefix)
	} else if room = MaxBlocksPayload(c.PacketSize); room < 1 {
		return fmt.Errorf("%w: get blocks reply needs %d bytes", ErrPacketTooSmall, blocksReplyFixed+1)
	}
	room = min(room, 0xFFFF)
	if !short && room >= a.largestBlock() {
		serverRoom, err := c.fetchWholeBlocks(addr, a, room)
		if err != nil || a.Complete() {
			return err
		}
		// The server sends smaller packets than this client: fetch the
		// rest in parts that fit its replies.
		room = serverRoom
	}
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
			r := Request{Operation: OperationGetBlocks, Address: addr, Offset: m.Packing[i].PackOffset + off, Length: uint16(n)}
			if short {
				r = Request{Operation: OperationGetBlockPart, Address: addr, PrefixLength: manifestPrefix,
					BlockHash: m.Blocks[i].BlockHash, BlockPrefixLength: blockPrefix, Offset: off, Length: uint16(n)}
			}
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
				off := req.Offset
				if !short {
					off -= m.Packing[i].PackOffset
				} else if r.Total != m.Packing[i].BlockLength {
					return nil, fmt.Errorf("%w: block %d of %d bytes, the manifest says %d", ErrPacket, i+1, r.Total, m.Packing[i].BlockLength)
				}
				if _, err := a.AcceptPart(i, off, r.Payload); err != nil {
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
					off := req.Offset + uint64(len(r.Payload))
					if !short {
						off -= m.Packing[i].PackOffset
					}
					add(i, off, rest)
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
// many whole blocks per request as fit in room bytes. When a reply holds
// less than a block, because the server's packet size is smaller, it stops
// after the round and returns the reply's length.
func (c *Client) fetchWholeBlocks(addr Hash, a *Assembly, room int) (int, error) {
	badReplies := map[uint64]int{}
	serverRoom := 0
	for round := 0; !a.Complete() && serverRoom == 0; round++ {
		if round == maxBlockRounds {
			return 0, errors.New("blocks still missing after the retries")
		}
		var requests []Request
		for _, r := range a.MissingRanges(room) {
			requests = append(requests, Request{Operation: OperationGetBlocks, Address: addr, Offset: r[0], Length: uint16(r[1])})
		}
		err := c.exchange(requests, func(req Request, r Reply) ([]Request, error) {
			if r.Offset != req.Offset {
				return nil, fmt.Errorf("%w: get blocks reply at %d, asked %d", ErrPacket, r.Offset, req.Offset)
			}
			if len(r.Payload) < int(req.Length) && len(r.Payload) < a.largestBlock() {
				serverRoom = max(1, len(r.Payload))
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
			if serverRoom > 0 || end >= want {
				return nil, nil
			}
			// A shorter reply holds whole blocks: ask again from the first
			// block it did not hold whole.
			for _, p := range a.manifest.Packing {
				if p.PackOffset >= req.Offset && p.PackOffset+uint64(p.BlockLength) > end {
					return []Request{{Operation: OperationGetBlocks, Address: addr, Offset: p.PackOffset, Length: uint16(want - p.PackOffset)}}, nil
				}
			}
			return nil, nil
		})
		if err != nil {
			return serverRoom, err
		}
	}
	return serverRoom, nil
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
	if !c.trusted(m.SignerPublicKey) {
		return nil, fmt.Errorf("%s: %w: %s", name, ErrUntrusted, m.SignerPublicKey.Hex())
	}
	files, err := c.FetchFiles(addr, m)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &FetchedProgram{Address: addr, Manifest: m, Files: files}, nil
}
