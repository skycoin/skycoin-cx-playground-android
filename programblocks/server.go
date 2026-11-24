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
	rep := Reply{Operation: req.Operation | ReplyFlag, RequestID: req.RequestID, Address: req.Address, Offset: req.Offset}
	if req.Operation == OperationListManifests {
		rep.Address, rep.Total = c.ListAddress, uint32(len(c.manifests[c.ListAddress].encoded))
	} else {
		cm, ok := c.manifests[req.Address]
		if !ok {
			return fail(ErrorCodeUnknownManifest, "unknown manifest "+req.Address.String())
		}
		switch req.Operation {
		case OperationGetManifest:
			b := cm.encoded
			if req.Offset >= uint64(len(b)) {
				return fail(ErrorCodeRangeOutside, "offset past the manifest")
			}
			n := min(uint64(req.Length), uint64(len(b))-req.Offset, uint64(MaxManifestPayload(packetSize)))
			rep.Total, rep.Payload = uint32(len(b)), b[req.Offset:req.Offset+n]
		case OperationGetBlockList:
			blocks := cm.manifest.Blocks
			if req.Offset < 1 || req.Offset > uint64(len(blocks)) {
				return fail(ErrorCodeRangeOutside, "block ID outside the manifest")
			}
			n := min(uint64(req.Length), uint64(len(blocks))-req.Offset+1, uint64(MaxBlockListEntries(packetSize)))
			rep.Length = uint16(n)
			for _, e := range blocks[req.Offset-1 : req.Offset-1+n] {
				rep.Payload = EncodeBlockEntry(rep.Payload, e)
			}
		case OperationGetBlocks:
			if req.Offset >= uint64(len(cm.pack)) {
				return fail(ErrorCodeRangeOutside, "offset past the linear packing")
			}
			n := min(uint64(req.Length), uint64(len(cm.pack))-req.Offset, uint64(MaxBlocksPayload(packetSize)))
			rep.Payload = cm.pack[req.Offset : req.Offset+n]
		}
	}
	b, err := EncodeReply(rep, packetSize)
	if err != nil {
		return fail(ErrorCodeMalformedRequest, err.Error())
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
