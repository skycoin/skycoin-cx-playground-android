package programblocks

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/skycoin/skycoin/src/cipher"
)

// Catalog holds signed manifests and their linear packings by address, with
// a manifest list of the programs.
type Catalog struct {
	ListAddress Hash
	manifests   map[Hash]catalogManifest
}

type catalogManifest struct {
	encoded  []byte
	manifest *Manifest
	pack     []byte
}

// NewCatalog signs a manifest for every program, keyed by program name with
// its files keyed by path, and a manifest list of them.
func NewCatalog(programs map[string]map[string][]byte, secret cipher.SecKey, date time.Time) (*Catalog, error) {
	c := &Catalog{manifests: map[Hash]catalogManifest{}}
	encoded := map[string][]byte{}
	for name, files := range programs {
		_, b, err := c.add(func() (*Manifest, []byte, error) { return Build(files, DefaultBlockSize, date) }, secret)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		encoded[name] = b
	}
	addr, _, err := c.add(func() (*Manifest, []byte, error) { return BuildManifestList(encoded, date) }, secret)
	if err != nil {
		return nil, fmt.Errorf("manifest list: %w", err)
	}
	c.ListAddress = addr
	return c, nil
}

func (c *Catalog) add(build func() (*Manifest, []byte, error), secret cipher.SecKey) (Hash, []byte, error) {
	m, pack, err := build()
	if err != nil {
		return Hash{}, nil, err
	}
	if err := m.Sign(secret); err != nil {
		return Hash{}, nil, err
	}
	b := m.Encode()
	addr := m.Address()
	c.manifests[addr] = catalogManifest{encoded: b, manifest: m, pack: pack}
	return addr, b, nil
}

// EmptyCatalog returns a catalog with no manifest and no manifest list, for
// a node that adds the programs it holds with AddVerified.
func EmptyCatalog() *Catalog { return &Catalog{manifests: map[Hash]catalogManifest{}} }

// AddVerified adds an encoded manifest and its linear packing, after
// checking the manifest and every block and file of the packing.
func (c *Catalog) AddVerified(encoded, pack []byte) (Hash, error) {
	m, addr, err := Decode(encoded, Hash{})
	if err != nil {
		return addr, err
	}
	if uint64(len(pack)) != m.PackLength {
		return addr, fmt.Errorf("packing of %d bytes, manifest says %d", len(pack), m.PackLength)
	}
	a := NewAssembly(m)
	if _, err := a.Accept(0, pack); err != nil {
		return addr, err
	}
	if _, err := a.Files(); err != nil {
		return addr, err
	}
	c.manifests[addr] = catalogManifest{encoded: encoded, manifest: m, pack: pack}
	return addr, nil
}

// Holds reports whether the catalog holds the manifest at addr.
func (c *Catalog) Holds(addr Hash) bool {
	_, ok := c.manifests[addr]
	return ok
}

// Item returns the encoded manifest and the linear packing held at addr.
func (c *Catalog) Item(addr Hash) (encoded, pack []byte, ok bool) {
	cm, ok := c.manifests[addr]
	return cm.encoded, cm.pack, ok
}

// CatalogFromDirectory serves every subdirectory of dir as a program named
// after the subdirectory, read with ReadProgramDirectory.
func CatalogFromDirectory(dir string, secret cipher.SecKey, date time.Time) (*Catalog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	programs := map[string]map[string][]byte{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, err := ReadProgramDirectory(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		programs[e.Name()] = files
	}
	if len(programs) == 0 {
		return nil, fmt.Errorf("%s holds no program directories", dir)
	}
	return NewCatalog(programs, secret, date)
}

// Addresses returns the address of every manifest the catalog holds, in
// byte order.
func (c *Catalog) Addresses() []Hash {
	var out []Hash
	for a := range c.manifests {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i][:]) < string(out[j][:]) })
	return out
}

// errorReply is the code and text of an error reply.
type errorReply struct {
	code ErrorCode
	text string
}

// findManifest returns the manifest whose address starts with the first n
// bytes of addr. A prefix that matches two manifests is refused.
func (c *Catalog) findManifest(addr Hash, n int) (catalogManifest, *errorReply) {
	if n == len(addr) {
		if cm, ok := c.manifests[addr]; ok {
			return cm, nil
		}
	} else {
		var found []catalogManifest
		for a, cm := range c.manifests {
			if a.HasPrefix(addr, n) {
				found = append(found, cm)
			}
		}
		if len(found) > 1 {
			return catalogManifest{}, &errorReply{ErrorCodeAmbiguousPrefix, fmt.Sprintf("%d manifests start with %x", len(found), addr[:n])}
		}
		if len(found) == 1 {
			return found[0], nil
		}
	}
	return catalogManifest{}, &errorReply{ErrorCodeUnknownManifest, fmt.Sprintf("unknown manifest %x", addr[:n])}
}

// findBlock returns the bytes of the block whose hash starts with the
// block prefix of req, in the manifest req names by prefix or, with no
// manifest prefix, in any manifest. Blocks with the same hash hold the
// same bytes, so only a prefix matching two different hashes is refused.
func (c *Catalog) findBlock(req Request) ([]byte, *errorReply) {
	search := c.manifests
	if req.PrefixLength > 0 {
		cm, e := c.findManifest(req.Address, req.PrefixLength)
		if e != nil {
			return nil, e
		}
		search = map[Hash]catalogManifest{{}: cm}
	}
	var block []byte
	var blockHash Hash
	for _, cm := range search {
		for i, e := range cm.manifest.Blocks {
			if !e.BlockHash.HasPrefix(req.BlockHash, req.BlockPrefixLength) {
				continue
			}
			if block != nil && e.BlockHash != blockHash {
				return nil, &errorReply{ErrorCodeAmbiguousPrefix, fmt.Sprintf("two blocks start with %x", req.BlockHash[:req.BlockPrefixLength])}
			}
			p := cm.manifest.Packing[i]
			block, blockHash = cm.pack[p.PackOffset:p.PackOffset+uint64(p.BlockLength)], e.BlockHash
		}
	}
	if block == nil {
		return nil, &errorReply{ErrorCodeUnknownManifest, fmt.Sprintf("no block starts with %x", req.BlockHash[:req.BlockPrefixLength])}
	}
	return block, nil
}

// blockListBytes returns the block list of cm as 48-byte entries, the
// form get block list bytes reads in pieces.
func blockListBytes(cm catalogManifest) []byte { return encodeBlockList(cm.manifest.Blocks) }

// Reply answers one request packet. The reply never exceeds packetSize. It
// returns nil for a packet too short to answer.
func (c *Catalog) Reply(packet []byte, packetSize int) []byte {
	req, err := DecodeRequest(packet)
	fail := func(code ErrorCode, text string) []byte {
		b, _ := EncodeReply(Reply{Operation: OperationError, RequestID: req.RequestID, ErrorCode: code, ErrorText: text}, packetSize)
		return b
	}
	if err != nil {
		if len(packet) < PacketHeaderLength {
			return nil
		}
		if packet[0] != ProtocolVersion {
			return fail(ErrorCodeUnsupportedVersion, err.Error())
		}
		return fail(ErrorCodeMalformedRequest, err.Error())
	}
	tooSmall := func(room int) []byte {
		return fail(ErrorCodePacketTooSmall, fmt.Sprintf("reply holds %d bytes", room))
	}
	// part returns at most room bytes of b from req.Offset.
	part := func(b []byte, room int) ([]byte, *errorReply) {
		if req.Offset >= uint64(len(b)) {
			return nil, &errorReply{ErrorCodeRangeOutside, fmt.Sprintf("offset %d past %d bytes", req.Offset, len(b))}
		}
		n := min(uint64(req.Length), uint64(len(b))-req.Offset, uint64(room))
		return b[req.Offset : req.Offset+n], nil
	}
	rep := Reply{Operation: req.Operation | ReplyFlag, RequestID: req.RequestID, Address: req.Address, PrefixLength: req.PrefixLength, Offset: req.Offset}
	switch req.Operation {
	case OperationListManifests, OperationListManifestsPrefix:
		rep.Address, rep.Total = c.ListAddress, uint32(len(c.manifests[c.ListAddress].encoded))
	case OperationGetBlockPart:
		room := MaxBlockPartPayload(packetSize, req.BlockPrefixLength)
		if room < 1 {
			return tooSmall(room)
		}
		block, e := c.findBlock(req)
		if e == nil && len(block) > 0xFFFF {
			e = &errorReply{ErrorCodeMalformedRequest, "block longer than a get block part can name"}
		}
		if e == nil {
			rep.Payload, e = part(block, room)
		}
		if e != nil {
			return fail(e.code, e.text)
		}
		rep.BlockHash, rep.BlockPrefixLength, rep.Total = req.BlockHash, req.BlockPrefixLength, uint32(len(block))
	default:
		cm, e := c.findManifest(req.Address, req.PrefixLength)
		if e != nil {
			return fail(e.code, e.text)
		}
		switch req.Operation {
		case OperationGetManifest, OperationGetManifestByPrefix, OperationGetBlockListBytes:
			b, room := cm.encoded, MaxManifestPayload(packetSize)
			if req.Operation != OperationGetManifest {
				room = MaxByPrefixPayload(packetSize, req.PrefixLength)
			}
			if req.Operation == OperationGetBlockListBytes {
				b = blockListBytes(cm)
			}
			if room < 1 {
				return tooSmall(room)
			}
			if rep.Payload, e = part(b, room); e != nil {
				return fail(e.code, e.text)
			}
			rep.Total = uint32(len(b))
		case OperationGetBlockList:
			blocks := cm.manifest.Blocks
			if req.Offset < 1 || req.Offset > uint64(len(blocks)) {
				return fail(ErrorCodeRangeOutside, "block ID outside the manifest")
			}
			room := MaxBlockListEntries(packetSize)
			if room < 1 {
				return tooSmall(packetSize - blockListReplyFixed)
			}
			n := min(uint64(req.Length), uint64(len(blocks))-req.Offset+1, uint64(room))
			rep.Length = uint16(n)
			for _, e := range blocks[req.Offset-1 : req.Offset-1+n] {
				rep.Payload = EncodeBlockEntry(rep.Payload, e)
			}
		case OperationGetBlocks:
			room := MaxBlocksPayload(packetSize)
			if room < 1 {
				return tooSmall(room)
			}
			if rep.Payload, e = part(cm.pack, room); e != nil {
				return fail(e.code, e.text)
			}
		}
	}
	b, err := EncodeReply(rep, packetSize)
	if err != nil {
		return fail(ErrorCodePacketTooSmall, err.Error())
	}
	return b
}

// Serve answers requests on conn until it is closed.
func (c *Catalog) Serve(conn net.PacketConn, packetSize int) error {
	if err := CheckPacketSize(packetSize); err != nil {
		return err
	}
	buf := make([]byte, 2048)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if reply := c.Reply(buf[:n], packetSize); reply != nil {
			conn.WriteTo(reply, from)
		}
	}
}
