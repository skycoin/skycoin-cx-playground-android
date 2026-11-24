// Package programtransportshortforms checks the short forms of #31: each
// request and reply has the length docs/ProgramDistribution.md gives, a
// block is read by a hash prefix with or without a manifest prefix and by
// a range inside it, a prefix that matches two different blocks is
// refused, and a client and server of different packet sizes still fetch
// a program.
package programtransportshortforms

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

var date = time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

func TestPacketLengths(t *testing.T) {
	addr := programblocks.Sum([]byte("manifest"))
	block := programblocks.Sum([]byte("block"))
	for _, c := range []struct {
		request programblocks.Request
		length  int
	}{
		{programblocks.Request{Operation: programblocks.OperationListManifestsPrefix, PrefixLength: 8}, 7},
		{programblocks.Request{Operation: programblocks.OperationGetManifestByPrefix, Address: addr, PrefixLength: 8, Offset: 300, Length: 50}, 21},
		{programblocks.Request{Operation: programblocks.OperationGetBlockListBytes, Address: addr, PrefixLength: 4, Offset: 96, Length: 48}, 17},
		{programblocks.Request{Operation: programblocks.OperationGetBlockPart, Address: addr, PrefixLength: 8, BlockHash: block, BlockPrefixLength: 8, Offset: 500, Length: 12}, 28},
		{programblocks.Request{Operation: programblocks.OperationGetBlockPart, BlockHash: block, BlockPrefixLength: 4, Offset: 0, Length: 1}, 16},
	} {
		b := programblocks.EncodeRequest(c.request)
		if len(b) != c.length || programblocks.RequestLength(c.request) != c.length {
			t.Errorf("operation %#x: %d bytes, want %d", byte(c.request.Operation), len(b), c.length)
		}
		got, err := programblocks.DecodeRequest(b)
		if err != nil {
			t.Fatal(err)
		}
		want := c.request
		var prefix, blockPrefix programblocks.Hash
		copy(prefix[:want.PrefixLength], want.Address[:want.PrefixLength])
		copy(blockPrefix[:want.BlockPrefixLength], want.BlockHash[:want.BlockPrefixLength])
		want.Address, want.BlockHash = prefix, blockPrefix
		if got != want {
			t.Errorf("round trip: got %+v, want %+v", got, want)
		}
	}
	for _, c := range []struct {
		reply  programblocks.Reply
		length int
	}{
		{programblocks.Reply{Operation: programblocks.OperationListManifestsPrefix | programblocks.ReplyFlag, Address: addr, PrefixLength: 8, Total: 703}, 19},
		{programblocks.Reply{Operation: programblocks.OperationGetManifestByPrefix | programblocks.ReplyFlag, Address: addr, PrefixLength: 8, Total: 703, Payload: make([]byte, 105)}, 128},
		{programblocks.Reply{Operation: programblocks.OperationGetBlockPart | programblocks.ReplyFlag, BlockHash: block, BlockPrefixLength: 8, Offset: 400, Total: 512, Payload: make([]byte, 109)}, 128},
	} {
		b, err := programblocks.EncodeReply(c.reply, 128)
		if err != nil || len(b) != c.length {
			t.Fatalf("operation %#x: %d bytes, %v, want %d", byte(c.reply.Operation), len(b), err, c.length)
		}
		got, err := programblocks.DecodeReply(b)
		if err != nil || got.Total != c.reply.Total || got.Offset != c.reply.Offset || len(got.Payload) != len(c.reply.Payload) {
			t.Fatalf("operation %#x round trip: %+v, %v", byte(c.reply.Operation), got, err)
		}
	}
	// Prefixes shorter than 4 bytes or longer than 32 are malformed, and
	// only a get block part may leave out the manifest.
	for _, b := range [][]byte{
		{1, 0x05, 0, 0, 0, 1, 3},
		{1, 0x05, 0, 0, 0, 1, 33},
		{1, 0x06, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 1},
		{1, 0x08, 0, 0, 0, 1, 0, 3, 1, 2, 3, 0, 0, 0, 1},
	} {
		if _, err := programblocks.DecodeRequest(b); err == nil {
			t.Errorf("request % x accepted", b)
		}
	}
}

// collidingBlocks returns two different texts whose SHA-256 hashes share
// their first 4 bytes, found by a birthday search.
func collidingBlocks(t *testing.T) ([]byte, []byte) {
	seen := map[[4]byte][]byte{}
	for i := 0; i < 1<<22; i++ {
		b := []byte(fmt.Sprintf("block %d\n", i))
		h := programblocks.Sum(b)
		key := [4]byte(h[:4])
		if other, ok := seen[key]; ok {
			return other, b
		}
		seen[key] = b
	}
	t.Fatal("no 4-byte collision found")
	return nil, nil
}

type server struct {
	catalog *programblocks.Catalog
	signer  cipher.PubKey
	address programblocks.Hash
}

func newServer(t *testing.T, files map[string][]byte) server {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("short forms"))
	c, err := programblocks.NewCatalog(map[string]map[string][]byte{"program": files}, secret, date)
	if err != nil {
		t.Fatal(err)
	}
	s := server{catalog: c, signer: pub}
	for _, a := range c.Addresses() {
		if a != c.ListAddress {
			s.address = a
		}
	}
	return s
}

func (s server) ask(t *testing.T, r programblocks.Request, size int) programblocks.Reply {
	t.Helper()
	b := s.catalog.Reply(programblocks.EncodeRequest(r), size)
	if len(b) > size {
		t.Fatalf("reply of %d bytes at packet size %d", len(b), size)
	}
	rep, err := programblocks.DecodeReply(b)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func TestBlockByPrefix(t *testing.T) {
	first, second := collidingBlocks(t)
	main := bytes.Repeat([]byte("0123456789abcdef"), 40)
	s := newServer(t, map[string][]byte{"a.txt": first, "b.txt": second, "c.txt": first, "main.cx": main})
	firstHash, mainHash := programblocks.Sum(first), programblocks.Sum(main[:512])
	t.Logf("%q and %q share the hash prefix %x", first, second, firstHash[:4])
	part := func(manifestPrefix, blockPrefix int, h programblocks.Hash, off, n int) programblocks.Request {
		return programblocks.Request{Operation: programblocks.OperationGetBlockPart, Address: s.address, PrefixLength: manifestPrefix,
			BlockHash: h, BlockPrefixLength: blockPrefix, Offset: uint64(off), Length: uint16(n)}
	}
	// A range inside the first block of main.cx, by block prefix alone and
	// with the manifest prefix.
	for _, mp := range []int{0, 4, 8, 32} {
		r := s.ask(t, part(mp, 8, mainHash, 100, 50), 128)
		if r.Operation != programblocks.OperationGetBlockPart|programblocks.ReplyFlag || r.Total != 512 || r.Offset != 100 || !bytes.Equal(r.Payload, main[100:150]) {
			t.Fatalf("manifest prefix %d: %+v", mp, r)
		}
	}
	// A reply is cut to the packet size.
	if r := s.ask(t, part(4, 8, mainHash, 0, 512), 64); len(r.Payload) != programblocks.MaxBlockPartPayload(64, 8) {
		t.Fatalf("payload of %d bytes at 64", len(r.Payload))
	}
	// Two blocks of different bytes share 4 bytes: refused at 4, served at
	// 8. a.txt and c.txt are the same block, which is not ambiguous.
	if r := s.ask(t, part(0, 4, firstHash, 0, 100), 128); r.ErrorCode != programblocks.ErrorCodeAmbiguousPrefix {
		t.Fatalf("4-byte colliding prefix: %+v", r)
	}
	if r := s.ask(t, part(0, 8, firstHash, 0, 100), 128); !bytes.Equal(r.Payload, first) {
		t.Fatalf("8-byte prefix: %+v", r)
	}
	for _, c := range []struct {
		request programblocks.Request
		code    programblocks.ErrorCode
	}{
		{part(0, 8, programblocks.Sum([]byte("nothing")), 0, 1), programblocks.ErrorCodeUnknownManifest},
		{part(8, 8, mainHash, 512, 1), programblocks.ErrorCodeRangeOutside},
		{programblocks.Request{Operation: programblocks.OperationGetManifestByPrefix, Address: programblocks.Sum([]byte("x")), PrefixLength: 8, Length: 10}, programblocks.ErrorCodeUnknownManifest},
	} {
		if r := s.ask(t, c.request, 128); r.Operation != programblocks.OperationError || r.ErrorCode != c.code {
			t.Errorf("request %+v: reply %+v, want error %d", c.request, r, c.code)
		}
	}
	// A reply whose fixed part does not fit says so.
	if r := s.ask(t, programblocks.Request{Operation: programblocks.OperationGetBlockList, Address: s.address, Offset: 1, Length: 1}, 64); r.ErrorCode != programblocks.ErrorCodePacketTooSmall {
		t.Fatalf("get block list at 64: %+v", r)
	}
}

// TestMismatchedPacketSizes fetches a program with each forms flag from
// servers of other packet sizes. The short forms work with every pair. The
// full forms work only when both sides hold a whole block, and otherwise
// fail with error 6 naming the forms flag (D37).
func TestMismatchedPacketSizes(t *testing.T) {
	files, err := programblocks.ReadProgramDirectory("../../apps/benchmarks")
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(t, files)
	for _, serverSize := range []int{1200, 128, 64} {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go s.catalog.Serve(pc, serverSize)
		for _, forms := range []programblocks.Forms{programblocks.FormsFull, programblocks.FormsShort} {
			for _, clientSize := range []int{1400, 1200, 512, 128, 64, 32} {
				conn, err := net.Dial("udp", pc.LocalAddr().String())
				if err != nil {
					t.Fatal(err)
				}
				c := programblocks.NewClient(conn, s.signer)
				c.PacketSize, c.Forms = clientSize, forms
				p, err := c.FetchProgram("program")
				conn.Close()
				fits := forms == programblocks.FormsShort || min(serverSize, clientSize) >= programblocks.MinFullFormPacketSize
				switch {
				case fits && err != nil:
					t.Fatalf("server %d, client %d, forms=%s: %v", serverSize, clientSize, forms, err)
				case !fits && (!errors.Is(err, programblocks.ErrPacketTooSmall) || !strings.Contains(err.Error(), "forms flag")):
					t.Fatalf("server %d, client %d, forms=%s: %v, want error 6 naming the forms flag", serverSize, clientSize, forms, err)
				case fits && !bytes.Equal(p.Files["primes.cx"], files["primes.cx"]):
					t.Fatalf("server %d, client %d, forms=%s: wrong files", serverSize, clientSize, forms)
				case fits:
					t.Logf("server %d, client %d, forms=%s: %d requests, largest reply %d bytes", serverSize, clientSize, forms, c.Stats.RequestsSent, c.Stats.LargestReply)
				default:
					t.Logf("server %d, client %d, forms=%s: %v", serverSize, clientSize, forms, err)
				}
			}
		}
		pc.Close()
	}
}
