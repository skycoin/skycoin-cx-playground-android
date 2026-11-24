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

// Client fetches and verifies programs from a server over a connected UDP
// socket.
type Client struct {
	Conn           net.Conn
	PacketSize     int
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
	BytesReceived    int
	LargestReply     int
	RangesRejected   int // get blocks replies that failed a block hash
}

// NewClient returns a client with the default settings that accepts
// manifests signed by any of trusted.
func NewClient(conn net.Conn, trusted ...cipher.PubKey) *Client {
	return &Client{Conn: conn, PacketSize: DefaultPacketSize, Window: DefaultWindow, FirstTimeout: DefaultFirstTimeout,
		MaxTimeout: DefaultMaxTimeout, MaxTries: DefaultMaxTries, TrustedSigners: trusted, nextRequestID: rand.Uint32()}
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
		_, err := c.Conn.Write(EncodeRequest(f.request))
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
		if reply.Operation != f.request.Operation|ReplyFlag || (reply.Operation != OperationListManifests|ReplyFlag && reply.Address != f.request.Address) {
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

// ListManifests returns the address and length of the server's manifest list.
func (c *Client) ListManifests() (Hash, uint32, error) {
	var addr Hash
	var length uint32
	err := c.exchange([]Request{{Operation: OperationListManifests}}, func(_ Request, r Reply) ([]Request, error) {
		addr, length = r.Address, r.Total
		return nil, nil
	})
	return addr, length, err
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
// checks the address, signature and signer from the first piece, each array
// as soon as it is complete, and the whole manifest at the end.
func (c *Client) FetchManifest(addr Hash) (*Manifest, []byte, error) {
	piece := MaxManifestPayload(c.PacketSize)
	var buf []byte
	var have []bool
	var header *Header
	verified := map[int]bool{}
	ask := func(off, n int) Request {
		return Request{Operation: OperationGetManifest, Address: addr, Offset: uint64(off), Length: uint16(n)}
	}
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
				if got != addr {
					return nil, fmt.Errorf("%w: got %s, want %s", ErrAddress, got, addr)
				}
				if !c.trusted(h.SignerPublicKey) {
					return nil, fmt.Errorf("%w: %s", ErrUntrusted, h.SignerPublicKey.Hex())
				}
				header = &h
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
		return nil, nil, err
	}
	m, _, err := Decode(buf, addr)
	return m, buf, err
}

func allTrue(b []bool) bool {
	for _, v := range b {
		if !v {
			return false
		}
	}
	return true
}

// FetchFiles fetches the linear packing of m, the manifest at addr, two
// blocks per packet, verifies every block and file and returns the files.
func (c *Client) FetchFiles(addr Hash, m *Manifest) (map[string][]byte, error) {
	a := NewAssembly(m)
	badReplies := map[uint64]int{}
	for round := 0; !a.Complete(); round++ {
		if round == maxBlockRounds {
			return nil, errors.New("blocks still missing after the retries")
		}
		var requests []Request
		for _, r := range a.MissingRanges(MaxBlocksPayload(c.PacketSize)) {
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
			return nil, nil
		})
		if err != nil {
			return nil, err
		}
	}
	return a.Files()
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
	listAddr, _, err := c.ListManifests()
	if err != nil {
		return nil, err
	}
	list, _, err := c.FetchManifest(listAddr)
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
