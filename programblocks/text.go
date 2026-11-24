package programblocks

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	textsections "github.com/skycoin/skycoin-cx/t0-textsections"
)

// The data of a manifest is the text sections form of skycoin-cx
// t0-textsections (D24): a header line naming a section and its columns,
// one entry per line, and a blank line before the next section. See "Text
// form" in docs/ProgramDistribution.md. Offsets, block IDs and the packing
// map follow from the sizes, so the text leaves them out.
var (
	textSectionManifest = textsections.TextSection{TextSectionName: "manifest", TextSectionColumns: []string{"date", "pack_length", "merkle_hash"}}
	textSectionFiles    = textsections.TextSection{TextSectionName: "files", TextSectionColumns: []string{"file_hash", "size", "block_size", "merkle_hash", "path"}}
	textSectionBlocks   = textsections.TextSection{TextSectionName: "blocks", TextSectionColumns: []string{"block_hash", "size"}}
)

// EncodeText returns the text form of m, the data of its envelope before
// any compression.
func (m *Manifest) EncodeText() []byte {
	sections := []textsections.TextSection{textSectionManifest, textSectionFiles, textSectionBlocks}
	entry := func(fields ...string) textsections.TextSectionEntry {
		return textsections.TextSectionEntry{TextSectionEntryText: strings.Join(fields, " ")}
	}
	sections[0].TextSectionEntries = []textsections.TextSectionEntry{entry(u64(m.Date), u64(m.PackLength), m.MerkleHash.String())}
	sections[1].TextSectionEntries = make([]textsections.TextSectionEntry, 0, len(m.Files))
	for _, f := range m.Files {
		sections[1].TextSectionEntries = append(sections[1].TextSectionEntries, entry(f.FileHash.String(), u64(f.FileLength), u32(f.BlockSize), f.MerkleHash.String(), f.Path))
	}
	sections[2].TextSectionEntries = make([]textsections.TextSectionEntry, 0, len(m.Blocks))
	for _, b := range m.Blocks {
		sections[2].TextSectionEntries = append(sections[2].TextSectionEntries, entry(b.BlockHash.String(), u32(b.BlockLength)))
	}
	return textsections.TextSectionsWrite(nil, sections)
}

func u64(v uint64) string { return strconv.FormatUint(v, 10) }
func u32(v uint32) string { return strconv.FormatUint(uint64(v), 10) }

// DecodeText parses the text form, rebuilds the offsets, block IDs and
// packing map, and checks the sizes and Merkle hashes. The hashes of the
// files and blocks are checked when their bytes arrive.
func DecodeText(text []byte) (*Manifest, error) {
	sections, err := textsections.TextSectionsRead(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	want := []textsections.TextSection{textSectionManifest, textSectionFiles, textSectionBlocks}
	if len(sections) != len(want) {
		return nil, fmt.Errorf("%w: %d sections, want %d", ErrMalformed, len(sections), len(want))
	}
	for i, s := range sections {
		if s.TextSectionName != want[i].TextSectionName || strings.Join(s.TextSectionColumns, " ") != strings.Join(want[i].TextSectionColumns, " ") {
			return nil, fmt.Errorf("%w: section %d is [%s] %s, want [%s] %s", ErrMalformed, i+1, s.TextSectionName,
				strings.Join(s.TextSectionColumns, " "), want[i].TextSectionName, strings.Join(want[i].TextSectionColumns, " "))
		}
	}
	p := &textFieldParser{}
	m := &Manifest{}
	if n := len(sections[0].TextSectionEntries); n != 1 {
		return nil, fmt.Errorf("%w: [manifest] has %d entries, want 1", ErrMalformed, n)
	}
	header := p.fields(sections[0].TextSectionEntries[0], 3)
	if header != nil {
		m.Date, m.PackLength, m.MerkleHash = p.u64(header[0]), p.u64(header[1]), p.hash(header[2])
	}
	var pack uint64
	next := uint32(1)
	for _, e := range sections[1].TextSectionEntries {
		f := p.fields(e, 5)
		if f == nil {
			break
		}
		entry := FileEntry{FileHash: p.hash(f[0]), FileLength: p.u64(f[1]), BlockSize: p.u32(f[2]), MerkleHash: p.hash(f[3]), Path: f[4], PackOffset: pack, FirstBlockID: next}
		if entry.BlockSize == 0 || entry.BlockSize > MaxBlockSize {
			p.fail(e, "block size %d", entry.BlockSize)
			break
		}
		entry.BlockCount = uint32((entry.FileLength + uint64(entry.BlockSize) - 1) / uint64(entry.BlockSize))
		m.Files = append(m.Files, entry)
		pack += entry.FileLength
		next += entry.BlockCount
	}
	for i, e := range sections[2].TextSectionEntries {
		f := p.fields(e, 2)
		if f == nil {
			break
		}
		m.Blocks = append(m.Blocks, BlockEntry{BlockID: uint32(i + 1), BlockHash: p.hash(f[0]), BlockLength: p.u32(f[1])})
	}
	if p.err != nil {
		return nil, p.err
	}
	if int(next-1) != len(m.Blocks) {
		return nil, fmt.Errorf("%w: files cover %d blocks, [blocks] has %d", ErrMalformed, next-1, len(m.Blocks))
	}
	m.Packing = make([]BlockPacking, len(m.Blocks))
	for _, f := range m.Files {
		for j := uint32(0); j < f.BlockCount; j++ {
			b := &m.Blocks[f.FirstBlockID-1+j]
			b.FileOffset = uint64(j) * uint64(f.BlockSize)
			m.Packing[b.BlockID-1] = BlockPacking{BlockID: b.BlockID, PackOffset: f.PackOffset + b.FileOffset, BlockLength: b.BlockLength}
		}
	}
	if err := m.checkArrays(); err != nil {
		return nil, err
	}
	return m, nil
}

// textFieldParser parses the fields of entries and keeps the first error.
type textFieldParser struct {
	err  error
	line int
}

func (p *textFieldParser) fail(e textsections.TextSectionEntry, format string, args ...any) {
	if p.err == nil {
		p.err = fmt.Errorf("%w: text line %d: %s", ErrMalformed, e.TextSectionEntryLine, fmt.Sprintf(format, args...))
	}
}

// fields splits an entry at single spaces into n fields, the last taking
// the rest of the line, or returns nil after recording the error.
func (p *textFieldParser) fields(e textsections.TextSectionEntry, n int) []string {
	if p.err != nil {
		return nil
	}
	p.line = e.TextSectionEntryLine
	f := strings.SplitN(e.TextSectionEntryText, " ", n)
	if len(f) != n {
		p.fail(e, "%d fields, want %d", len(f), n)
		return nil
	}
	for _, s := range f {
		if s == "" {
			p.fail(e, "an empty field")
			return nil
		}
	}
	return f
}

func (p *textFieldParser) set(format string, args ...any) {
	if p.err == nil {
		p.err = fmt.Errorf("%w: text line %d: %s", ErrMalformed, p.line, fmt.Sprintf(format, args...))
	}
}

func (p *textFieldParser) u64(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil || (len(s) > 1 && s[0] == '0') {
		p.set("bad number %q", s)
	}
	return v
}

func (p *textFieldParser) u32(s string) uint32 {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil || (len(s) > 1 && s[0] == '0') {
		p.set("bad number %q", s)
	}
	return uint32(v)
}

func (p *textFieldParser) hash(s string) Hash {
	var h Hash
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(h) || strings.ToLower(s) != s {
		p.set("bad hash %q", s)
		return h
	}
	copy(h[:], b)
	return h
}
