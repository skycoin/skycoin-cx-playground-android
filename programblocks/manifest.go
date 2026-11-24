// Package programblocks is the signed program manifest of
// docs/ProgramDistribution.md: files split into 512-byte blocks, all blocks
// laid out in one linear packing, a Merkle hash over the block hashes and a
// secp256k1 signature, with the UDP operations that fetch them.
package programblocks

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/skycoin/skycoin/src/cipher"
)

// Format constants, see "Manifest format" in docs/ProgramDistribution.md.
const (
	Magic                     = "CXPM"
	FormatVersion             = 1
	DefaultBlockSize          = 512
	MaxBlockSize              = 1 << 16
	HeaderFixedLength         = 64
	ArrayDirectoryEntryLength = 48
	PublicKeyLength           = 33
	SignatureLength           = 65
	FileEntryFixedLength      = 94
	BlockEntryLength          = 48
	BlockPackingEntryLength   = 16
	MaxPathLength             = 255
	MaxArrayCount             = 16
	// MaxManifestLength bounds what a reader accepts, about 20000 blocks.
	MaxManifestLength = 1 << 20
)

// ArrayKind is the kind of an array in the array directory.
type ArrayKind uint16

// The array kinds of format version 1, in the order they are written.
const (
	ArrayKindFileList        ArrayKind = 1
	ArrayKindBlockList       ArrayKind = 2
	ArrayKindBlockPackingMap ArrayKind = 3
	arrayKindCountVersion1             = 3
	headerLengthVersion1               = HeaderFixedLength + arrayKindCountVersion1*ArrayDirectoryEntryLength + PublicKeyLength + SignatureLength
	signedPartLengthVersion1           = headerLengthVersion1 - SignatureLength
	merkleInnerNodePrefix    byte      = 0x01
)

// HeaderLengthVersion1 is the header length with the three version 1 arrays.
const HeaderLengthVersion1 = headerLengthVersion1

// Hash is a SHA-256 digest.
type Hash [32]byte

// Sum returns the SHA-256 of b.
func Sum(b []byte) Hash { return sha256.Sum256(b) }

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

// ArrayDirectoryEntry describes one array of a manifest.
type ArrayDirectoryEntry struct {
	Kind      ArrayKind
	Count     uint32
	Offset    uint32
	Length    uint32
	ArrayHash Hash
}

// Header is the fixed part of a manifest, enough to check its address and
// signature before any array arrives.
type Header struct {
	Date            uint64
	ManifestLength  uint64
	PackLength      uint64
	MerkleHash      Hash
	Arrays          []ArrayDirectoryEntry
	SignerPublicKey cipher.PubKey
	Signature       cipher.Sig
}

// Manifest is a decoded manifest.
type Manifest struct {
	Header
	Files   []FileEntry
	Blocks  []BlockEntry
	Packing []BlockPacking
}

// Errors a reader returns, so a caller can tell the failures apart.
var (
	ErrTruncated  = errors.New("manifest truncated")
	ErrMalformed  = errors.New("manifest malformed")
	ErrSignature  = errors.New("manifest signature does not match the signer key")
	ErrArrayHash  = errors.New("manifest array hash mismatch")
	ErrMerkleHash = errors.New("manifest merkle hash mismatch")
	ErrAddress    = errors.New("manifest address is not the one asked for")
	ErrBlockHash  = errors.New("block hash mismatch")
	ErrFileHash   = errors.New("file hash mismatch")
	ErrUntrusted  = errors.New("manifest signer is not trusted")
)

// MerkleHash returns the Merkle hash of leaves: inner nodes are SHA-256 of
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
	m := &Manifest{Header: Header{Date: uint64(date.Unix())}}
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

// Sign sets the signer key and signature of m.
func (m *Manifest) Sign(secret cipher.SecKey) error {
	pub, err := cipher.PubKeyFromSecKey(secret)
	if err != nil {
		return err
	}
	m.SignerPublicKey = pub
	m.Signature = cipher.Sig{}
	addr := m.Address()
	sig, err := cipher.SignHash(cipher.SHA256(addr), secret)
	if err != nil {
		return err
	}
	m.Signature = sig
	return nil
}

// Address returns the manifest address: the SHA-256 of the signed part of
// the header, which holds the hash of every array.
func (m *Manifest) Address() Hash {
	b := m.Encode()
	return Sum(b[:signedPartLengthVersion1])
}

// Encode returns the bytes of m.
func (m *Manifest) Encode() []byte {
	arrays := [][]byte{encodeFileList(m.Files), encodeBlockList(m.Blocks), encodeBlockPackingMap(m.Packing)}
	kinds := []ArrayKind{ArrayKindFileList, ArrayKindBlockList, ArrayKindBlockPackingMap}
	counts := []int{len(m.Files), len(m.Blocks), len(m.Packing)}
	total := headerLengthVersion1
	for _, a := range arrays {
		total += len(a)
	}
	m.ManifestLength = uint64(total)
	m.Arrays = m.Arrays[:0]
	b := make([]byte, headerLengthVersion1, total)
	copy(b, Magic)
	be := binary.BigEndian
	be.PutUint16(b[4:], FormatVersion)
	be.PutUint16(b[6:], uint16(len(arrays)))
	be.PutUint64(b[8:], m.Date)
	be.PutUint64(b[16:], m.ManifestLength)
	be.PutUint64(b[24:], m.PackLength)
	copy(b[32:], m.MerkleHash[:])
	offset := headerLengthVersion1
	for i, a := range arrays {
		e := ArrayDirectoryEntry{Kind: kinds[i], Count: uint32(counts[i]), Offset: uint32(offset), Length: uint32(len(a)), ArrayHash: Sum(a)}
		m.Arrays = append(m.Arrays, e)
		d := b[HeaderFixedLength+i*ArrayDirectoryEntryLength:]
		be.PutUint16(d[0:], uint16(e.Kind))
		be.PutUint32(d[4:], e.Count)
		be.PutUint32(d[8:], e.Offset)
		be.PutUint32(d[12:], e.Length)
		copy(d[16:], e.ArrayHash[:])
		offset += len(a)
	}
	copy(b[signedPartLengthVersion1-PublicKeyLength:], m.SignerPublicKey[:])
	copy(b[signedPartLengthVersion1:], m.Signature[:])
	for _, a := range arrays {
		b = append(b, a...)
	}
	return b
}

func encodeFileList(files []FileEntry) []byte {
	var b []byte
	be := binary.BigEndian
	for _, f := range files {
		e := make([]byte, FileEntryFixedLength, FileEntryFixedLength+len(f.Path))
		copy(e[0:], f.FileHash[:])
		be.PutUint64(e[32:], f.FileLength)
		be.PutUint64(e[40:], f.PackOffset)
		be.PutUint32(e[48:], f.BlockSize)
		be.PutUint32(e[52:], f.FirstBlockID)
		be.PutUint32(e[56:], f.BlockCount)
		copy(e[60:], f.MerkleHash[:])
		be.PutUint16(e[92:], uint16(len(f.Path)))
		b = append(append(b, e...), f.Path...)
	}
	return b
}

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

func encodeBlockPackingMap(packing []BlockPacking) []byte {
	b := make([]byte, len(packing)*BlockPackingEntryLength)
	be := binary.BigEndian
	for i, e := range packing {
		x := b[i*BlockPackingEntryLength:]
		be.PutUint32(x[0:], e.BlockID)
		be.PutUint64(x[4:], e.PackOffset)
		be.PutUint32(x[12:], e.BlockLength)
	}
	return b
}

// HeaderLength returns the header length that the first bytes of a
// manifest announce, ErrTruncated when b is shorter than 8 bytes.
func HeaderLength(b []byte) (int, error) {
	if len(b) < 8 {
		return 0, ErrTruncated
	}
	if string(b[:4]) != Magic {
		return 0, fmt.Errorf("%w: bad magic", ErrMalformed)
	}
	if v := binary.BigEndian.Uint16(b[4:]); v != FormatVersion {
		return 0, fmt.Errorf("%w: format version %d", ErrMalformed, v)
	}
	k := int(binary.BigEndian.Uint16(b[6:]))
	if k == 0 || k > MaxArrayCount {
		return 0, fmt.Errorf("%w: %d arrays", ErrMalformed, k)
	}
	return HeaderFixedLength + k*ArrayDirectoryEntryLength + PublicKeyLength + SignatureLength, nil
}

// ParseHeader reads the header at the start of b, checks its signature and
// returns it with the manifest address. b may hold the whole manifest or
// just the first packet.
func ParseHeader(b []byte) (Header, Hash, error) {
	var h Header
	n, err := HeaderLength(b)
	if err != nil {
		return h, Hash{}, err
	}
	if len(b) < n {
		return h, Hash{}, ErrTruncated
	}
	be := binary.BigEndian
	h.Date = be.Uint64(b[8:])
	h.ManifestLength = be.Uint64(b[16:])
	h.PackLength = be.Uint64(b[24:])
	copy(h.MerkleHash[:], b[32:64])
	if h.ManifestLength < uint64(n) || h.ManifestLength > MaxManifestLength {
		return h, Hash{}, fmt.Errorf("%w: manifest length %d", ErrMalformed, h.ManifestLength)
	}
	k := (n - HeaderFixedLength - PublicKeyLength - SignatureLength) / ArrayDirectoryEntryLength
	next := uint64(n)
	for i := 0; i < k; i++ {
		d := b[HeaderFixedLength+i*ArrayDirectoryEntryLength:]
		e := ArrayDirectoryEntry{Kind: ArrayKind(be.Uint16(d[0:])), Count: be.Uint32(d[4:]), Offset: be.Uint32(d[8:]), Length: be.Uint32(d[12:])}
		copy(e.ArrayHash[:], d[16:48])
		if uint64(e.Offset) != next {
			return h, Hash{}, fmt.Errorf("%w: array %d at %d, want %d", ErrMalformed, i, e.Offset, next)
		}
		next += uint64(e.Length)
		h.Arrays = append(h.Arrays, e)
	}
	if next != h.ManifestLength {
		return h, Hash{}, fmt.Errorf("%w: arrays end at %d, manifest length %d", ErrMalformed, next, h.ManifestLength)
	}
	signed := n - SignatureLength
	copy(h.SignerPublicKey[:], b[signed-PublicKeyLength:signed])
	copy(h.Signature[:], b[signed:n])
	addr := Sum(b[:signed])
	if err := cipher.VerifyPubKeySignedHash(h.SignerPublicKey, h.Signature, cipher.SHA256(addr)); err != nil {
		return h, addr, fmt.Errorf("%w: %v", ErrSignature, err)
	}
	return h, addr, nil
}

// ArrayVerified reports whether the bytes of array e, inside the whole
// manifest b, match its hash.
func ArrayVerified(b []byte, e ArrayDirectoryEntry) bool {
	end := uint64(e.Offset) + uint64(e.Length)
	return end <= uint64(len(b)) && Sum(b[e.Offset:end]) == e.ArrayHash
}

// Decode parses and checks a whole manifest: its signature, every array
// hash, the agreement of the arrays and the Merkle hashes. want is the
// address it was asked for, or the zero Hash to accept any.
func Decode(b []byte, want Hash) (*Manifest, Hash, error) {
	h, addr, err := ParseHeader(b)
	if err != nil {
		return nil, addr, err
	}
	if want != (Hash{}) && addr != want {
		return nil, addr, fmt.Errorf("%w: got %s, want %s", ErrAddress, addr, want)
	}
	if uint64(len(b)) != h.ManifestLength {
		if uint64(len(b)) < h.ManifestLength {
			return nil, addr, ErrTruncated
		}
		return nil, addr, fmt.Errorf("%w: %d bytes, manifest length %d", ErrMalformed, len(b), h.ManifestLength)
	}
	m := &Manifest{Header: h}
	seen := map[ArrayKind]bool{}
	for _, e := range h.Arrays {
		if !ArrayVerified(b, e) {
			return nil, addr, fmt.Errorf("%w: kind %d", ErrArrayHash, e.Kind)
		}
		if seen[e.Kind] {
			return nil, addr, fmt.Errorf("%w: array kind %d twice", ErrMalformed, e.Kind)
		}
		seen[e.Kind] = true
		a := b[e.Offset : e.Offset+e.Length]
		switch e.Kind {
		case ArrayKindFileList:
			m.Files, err = decodeFileList(a, e.Count)
		case ArrayKindBlockList:
			m.Blocks, err = decodeFixed(a, e.Count, BlockEntryLength, DecodeBlockEntry)
		case ArrayKindBlockPackingMap:
			m.Packing, err = decodeFixed(a, e.Count, BlockPackingEntryLength, decodeBlockPacking)
		}
		if err != nil {
			return nil, addr, err
		}
	}
	for _, k := range []ArrayKind{ArrayKindFileList, ArrayKindBlockList, ArrayKindBlockPackingMap} {
		if !seen[k] {
			return nil, addr, fmt.Errorf("%w: no array of kind %d", ErrMalformed, k)
		}
	}
	if err := m.checkArrays(); err != nil {
		return nil, addr, err
	}
	return m, addr, nil
}

func decodeBlockPacking(b []byte) BlockPacking {
	be := binary.BigEndian
	return BlockPacking{BlockID: be.Uint32(b[0:]), PackOffset: be.Uint64(b[4:]), BlockLength: be.Uint32(b[12:])}
}

func decodeFixed[T any](a []byte, count uint32, size int, decode func([]byte) T) ([]T, error) {
	if uint64(len(a)) != uint64(count)*uint64(size) {
		return nil, fmt.Errorf("%w: array of %d bytes holds %d entries of %d", ErrMalformed, len(a), count, size)
	}
	out := make([]T, count)
	for i := range out {
		out[i] = decode(a[i*size:])
	}
	return out, nil
}

func decodeFileList(a []byte, count uint32) ([]FileEntry, error) {
	be := binary.BigEndian
	var files []FileEntry
	for i := uint32(0); i < count; i++ {
		if len(a) < FileEntryFixedLength {
			return nil, fmt.Errorf("%w: file list ends inside entry %d", ErrMalformed, i)
		}
		f := FileEntry{FileLength: be.Uint64(a[32:]), PackOffset: be.Uint64(a[40:]), BlockSize: be.Uint32(a[48:]), FirstBlockID: be.Uint32(a[52:]), BlockCount: be.Uint32(a[56:])}
		copy(f.FileHash[:], a[0:32])
		copy(f.MerkleHash[:], a[60:92])
		n := int(be.Uint16(a[92:]))
		if len(a) < FileEntryFixedLength+n {
			return nil, fmt.Errorf("%w: file list ends inside path %d", ErrMalformed, i)
		}
		f.Path = string(a[FileEntryFixedLength : FileEntryFixedLength+n])
		a = a[FileEntryFixedLength+n:]
		files = append(files, f)
	}
	if len(a) != 0 {
		return nil, fmt.Errorf("%w: %d bytes after the file list", ErrMalformed, len(a))
	}
	return files, nil
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

func validPath(p string) bool {
	return len(p) > 0 && len(p) <= MaxPathLength && fs.ValidPath(p) && p != "." &&
		path.Clean(p) == p && !strings.Contains(p, "\\")
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
// program name, each the binary form of a signed program manifest.
func BuildManifestList(programs map[string][]byte, date time.Time) (*Manifest, []byte, error) {
	return Build(programs, DefaultBlockSize, date)
}
