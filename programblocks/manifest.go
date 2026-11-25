// Package programblocks is the signed program manifest of
// docs/ProgramDistribution.md: files split into 512-byte blocks, all blocks
// laid out in one linear packing, a Merkle hash over the block hashes, the
// manifest in the text sections form inside a signed envelope, and the UDP
// operations that fetch them.
package programblocks

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/skycoin/skycoin-cx/t0-cxhash"
	"github.com/skycoin/skycoin/src/cipher"
)

// Format constants, see "Manifest format" in docs/ProgramDistribution.md.
const (
	Magic            = "CXPM"
	FormatVersion    = 3
	DefaultBlockSize = 512
	MaxBlockSize     = 1 << 16
	PublicKeyLength  = 33
	SignatureLength  = 65
	BlockEntryLength = 48
	MaxPathLength    = 255
	// MaxManifestLength bounds what a reader accepts, envelope and
	// uncompressed data alike, about 10000 blocks.
	MaxManifestLength = 1 << 20
	// HashName names the hash of Sum, the hash of CX (D23).
	HashName                   = cxhash.CxHashName
	merkleInnerNodePrefix byte = 0x01
)

// Hash is a digest of Sum.
type Hash [32]byte

// Sum returns the hash of b that names blocks, files and manifests, the
// hash of CX that cipher.HashOfBytes gives the loader, BLAKE3-256 (D23).
func Sum(b []byte) Hash { return cxhash.CxHashOfBytes(b) }

func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// HashFromHex parses 64 hex digits.
func HashFromHex(s string) (Hash, error) {
	var h Hash
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(h) {
		return h, fmt.Errorf("bad hash %q", s)
	}
	copy(h[:], b)
	return h, nil
}

// FileEntry is one entry of the file list. The file is the range PackOffset,
// FileLength of the linear packing, and its blocks are BlockCount block IDs
// from FirstBlockID.
type FileEntry struct {
	Path         string
	FileHash     Hash
	FileLength   uint64
	PackOffset   uint64
	BlockSize    uint32
	FirstBlockID uint32
	BlockCount   uint32
	MerkleHash   Hash
}

// BlockEntry is one entry of the block list.
type BlockEntry struct {
	BlockID     uint32
	BlockHash   Hash
	BlockLength uint32
	FileOffset  uint64
}

// BlockPacking is one entry of the block packing map: where a block sits in
// the linear packing.
type BlockPacking struct {
	BlockID     uint32
	PackOffset  uint64
	BlockLength uint32
}

// Manifest is a decoded manifest. Date, PackLength, MerkleHash and the three
// lists are its data, the text sections form. Compressed, Signature and
// SignerPublicKey belong to the envelope around the data.
type Manifest struct {
	Date       uint64
	PackLength uint64
	MerkleHash Hash
	Files      []FileEntry
	Blocks     []BlockEntry
	Packing    []BlockPacking
	// Compressed makes Encode send the data as one LZ4 block, marked #lz.
	Compressed bool
	Signature  cipher.Sig
	// SignerPublicKey is the key the signature recovers to. The envelope
	// does not carry it (D27 checks only that the signature matches).
	SignerPublicKey cipher.PubKey
}

// Errors a reader returns, so a caller can tell the failures apart.
var (
	ErrTruncated  = errors.New("manifest truncated")
	ErrMalformed  = errors.New("manifest malformed")
	ErrSignature  = errors.New("manifest signature does not match its hash")
	ErrDataHash   = errors.New("manifest data hash mismatch")
	ErrMerkleHash = errors.New("manifest merkle hash mismatch")
	ErrAddress    = errors.New("manifest address is not the one asked for")
	ErrBlockHash  = errors.New("block hash mismatch")
	ErrFileHash   = errors.New("file hash mismatch")
	ErrNotPinned  = errors.New("manifest signer is not a pinned signer")
)

// MerkleHash returns the Merkle hash of leaves: inner nodes are the Sum of
// 0x01, left and right, and an odd last node moves up unchanged.
func MerkleHash(leaves []Hash) Hash {
	if len(leaves) == 0 {
		return Sum(nil)
	}
	level := append([]Hash(nil), leaves...)
	var buf [1 + 2*len(Hash{})]byte
	buf[0] = merkleInnerNodePrefix
	for len(level) > 1 {
		next := level[:0]
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i])
				continue
			}
			copy(buf[1:], level[i][:])
			copy(buf[33:], level[i+1][:])
			next = append(next, Sum(buf[:]))
		}
		level = next
	}
	return level[0]
}

func blockHashes(blocks []BlockEntry) []Hash {
	hashes := make([]Hash, len(blocks))
	for i, b := range blocks {
		hashes[i] = b.BlockHash
	}
	return hashes
}

// Build makes the unsigned manifest of files, keyed by slash separated
// path, and returns it with its linear packing. Sign it before Encode.
func Build(files map[string][]byte, blockSize int, date time.Time) (*Manifest, []byte, error) {
	if blockSize <= 0 || blockSize > MaxBlockSize {
		return nil, nil, fmt.Errorf("block size %d out of range", blockSize)
	}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	m := &Manifest{Date: uint64(date.Unix())}
	var pack []byte
	for _, p := range paths {
		b := files[p]
		f := FileEntry{
			Path:         p,
			FileHash:     Sum(b),
			FileLength:   uint64(len(b)),
			PackOffset:   uint64(len(pack)),
			BlockSize:    uint32(blockSize),
			FirstBlockID: uint32(len(m.Blocks) + 1),
		}
		for off := 0; off < len(b); off += blockSize {
			block := b[off:min(off+blockSize, len(b))]
			id := uint32(len(m.Blocks) + 1)
			m.Blocks = append(m.Blocks, BlockEntry{BlockID: id, BlockHash: Sum(block), BlockLength: uint32(len(block)), FileOffset: uint64(off)})
			m.Packing = append(m.Packing, BlockPacking{BlockID: id, PackOffset: f.PackOffset + uint64(off), BlockLength: uint32(len(block))})
		}
		f.BlockCount = uint32(len(m.Blocks)) - f.FirstBlockID + 1
		f.MerkleHash = MerkleHash(blockHashes(m.Blocks[f.FirstBlockID-1:]))
		m.Files = append(m.Files, f)
		pack = append(pack, b...)
	}
	m.PackLength = uint64(len(pack))
	m.MerkleHash = MerkleHash(blockHashes(m.Blocks))
	if err := m.checkArrays(); err != nil {
		return nil, nil, err
	}
	return m, pack, nil
}

// Address returns the manifest address: the hash of the data bytes of its
// envelope, the LZ4 block when Compressed and the text otherwise (D30).
func (m *Manifest) Address() Hash { return Sum(m.Data()) }

// EncodeBlockEntry appends the 48 bytes of e to b.
func EncodeBlockEntry(b []byte, e BlockEntry) []byte {
	var x [BlockEntryLength]byte
	be := binary.BigEndian
	be.PutUint32(x[0:], e.BlockID)
	copy(x[4:], e.BlockHash[:])
	be.PutUint32(x[36:], e.BlockLength)
	be.PutUint64(x[40:], e.FileOffset)
	return append(b, x[:]...)
}

// DecodeBlockEntry reads one block list entry from the start of b.
func DecodeBlockEntry(b []byte) BlockEntry {
	be := binary.BigEndian
	e := BlockEntry{BlockID: be.Uint32(b[0:]), BlockLength: be.Uint32(b[36:]), FileOffset: be.Uint64(b[40:])}
	copy(e.BlockHash[:], b[4:36])
	return e
}

func encodeBlockList(blocks []BlockEntry) []byte {
	b := make([]byte, 0, len(blocks)*BlockEntryLength)
	for _, e := range blocks {
		b = EncodeBlockEntry(b, e)
	}
	return b
}

// checkArrays checks that the file list, block list and packing map agree
// with each other and with the Merkle hashes.
func (m *Manifest) checkArrays() error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrMalformed, fmt.Sprintf(format, args...))
	}
	if len(m.Blocks) != len(m.Packing) {
		return bad("%d blocks, %d packing entries", len(m.Blocks), len(m.Packing))
	}
	for i := range m.Blocks {
		id := uint32(i + 1)
		if m.Blocks[i].BlockID != id || m.Packing[i].BlockID != id {
			return bad("block %d out of order", id)
		}
	}
	var pack uint64
	next := uint32(1)
	for i, f := range m.Files {
		if !validPath(f.Path) {
			return bad("bad path %q", f.Path)
		}
		if i > 0 && m.Files[i-1].Path >= f.Path {
			return bad("paths not sorted or repeated at %q", f.Path)
		}
		if f.BlockSize == 0 || f.BlockSize > MaxBlockSize {
			return bad("%s: block size %d", f.Path, f.BlockSize)
		}
		count := (f.FileLength + uint64(f.BlockSize) - 1) / uint64(f.BlockSize)
		if f.PackOffset != pack || f.FirstBlockID != next || uint64(f.BlockCount) != count {
			return bad("%s: packing offset, first block or block count", f.Path)
		}
		if uint64(f.FirstBlockID-1)+count > uint64(len(m.Blocks)) {
			return bad("%s: blocks past the block list", f.Path)
		}
		blocks := m.Blocks[f.FirstBlockID-1 : uint64(f.FirstBlockID-1)+count]
		for j, blk := range blocks {
			off := uint64(j) * uint64(f.BlockSize)
			want := uint32(min(uint64(f.BlockSize), f.FileLength-off))
			p := m.Packing[blk.BlockID-1]
			if blk.FileOffset != off || blk.BlockLength != want || p.PackOffset != f.PackOffset+off || p.BlockLength != want {
				return bad("%s: block %d offset or length", f.Path, blk.BlockID)
			}
		}
		if MerkleHash(blockHashes(blocks)) != f.MerkleHash {
			return fmt.Errorf("%w: file %s", ErrMerkleHash, f.Path)
		}
		pack += f.FileLength
		next += f.BlockCount
	}
	if int(next-1) != len(m.Blocks) || pack != m.PackLength {
		return bad("files cover %d blocks and %d bytes, manifest has %d and %d", next-1, pack, len(m.Blocks), m.PackLength)
	}
	if MerkleHash(blockHashes(m.Blocks)) != m.MerkleHash {
		return ErrMerkleHash
	}
	return nil
}

// validPath reports whether p is a clean relative path that fits on one
// line of the [files] section.
func validPath(p string) bool {
	return len(p) > 0 && len(p) <= MaxPathLength && fs.ValidPath(p) && p != "." &&
		path.Clean(p) == p && !strings.ContainsAny(p, "\\\r\n") && strings.TrimSpace(p) == p
}

// BlockAt returns the index in m.Blocks of the block that starts at pack
// offset off, or -1.
func (m *Manifest) BlockAt(off uint64) int {
	i := sort.Search(len(m.Packing), func(i int) bool { return m.Packing[i].PackOffset >= off })
	if i < len(m.Packing) && m.Packing[i].PackOffset == off {
		return i
	}
	return -1
}

// BuildManifestList makes the unsigned manifest list of programs, keyed by
// program name, each the envelope of a signed program manifest.
func BuildManifestList(programs map[string][]byte, date time.Time) (*Manifest, []byte, error) {
	return Build(programs, DefaultBlockSize, date)
}
