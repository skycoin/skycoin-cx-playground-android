package programblocks

import (
	"fmt"
)

// Assembly collects the linear packing of a verified manifest from ranges
// that arrive in any order, and checks every block as it arrives.
type Assembly struct {
	manifest *Manifest
	pack     []byte
	received []bool // per block, in block ID order
	missing  int
	// BlockStored, when set, is called with each new block after its hash
	// matched, before it counts as received, so a store can write it out.
	BlockStored func(packOffset uint64, block []byte) error
}

// NewAssembly starts the assembly of the linear packing of m, which must
// come from Decode or Build.
func NewAssembly(m *Manifest) *Assembly {
	return &Assembly{manifest: m, pack: make([]byte, m.PackLength), received: make([]bool, len(m.Blocks)), missing: len(m.Blocks)}
}

// Accept stores the whole blocks inside the linear packing range that
// starts at offset, checking each against its hash. Partial blocks at either
// end are ignored. It returns how many new blocks it stored, and
// ErrBlockHash for the first block that does not match.
func (a *Assembly) Accept(offset uint64, data []byte) (int, error) {
	m := a.manifest
	i := m.BlockAt(offset)
	if i < 0 {
		// The range starts inside a block: skip to the next block start.
		for i = 0; i < len(m.Packing) && m.Packing[i].PackOffset < offset; i++ {
		}
	}
	stored := 0
	end := offset + uint64(len(data))
	for ; i < len(m.Packing); i++ {
		p := m.Packing[i]
		if p.PackOffset+uint64(p.BlockLength) > end {
			break
		}
		block := data[p.PackOffset-offset : p.PackOffset-offset+uint64(p.BlockLength)]
		if Sum(block) != m.Blocks[i].BlockHash {
			return stored, fmt.Errorf("%w: block %d at offset %d", ErrBlockHash, p.BlockID, p.PackOffset)
		}
		if !a.received[i] {
			if a.BlockStored != nil {
				if err := a.BlockStored(p.PackOffset, block); err != nil {
					return stored, err
				}
			}
			copy(a.pack[p.PackOffset:], block)
			a.received[i] = true
			a.missing--
			stored++
		}
	}
	return stored, nil
}

// Complete reports whether every block is in.
func (a *Assembly) Complete() bool { return a.missing == 0 }

// MissingBlocks returns the number of blocks not in yet.
func (a *Assembly) MissingBlocks() int { return a.missing }

// MissingRanges returns the ranges of the linear packing still missing, as
// offset and length pairs of whole blocks, each at most maxLength bytes
// unless a single block is longer.
func (a *Assembly) MissingRanges(maxLength int) [][2]uint64 {
	var out [][2]uint64
	m := a.manifest
	for i := 0; i < len(m.Packing); i++ {
		if a.received[i] {
			continue
		}
		start := m.Packing[i].PackOffset
		length := uint64(m.Packing[i].BlockLength)
		for i+1 < len(m.Packing) && !a.received[i+1] && length+uint64(m.Packing[i+1].BlockLength) <= uint64(maxLength) {
			i++
			length += uint64(m.Packing[i].BlockLength)
		}
		out = append(out, [2]uint64{start, length})
	}
	return out
}

// Files returns every file once all blocks are in, after checking each
// file's length and hash.
func (a *Assembly) Files() (map[string][]byte, error) {
	if !a.Complete() {
		return nil, fmt.Errorf("%d blocks missing", a.missing)
	}
	files := make(map[string][]byte, len(a.manifest.Files))
	for _, f := range a.manifest.Files {
		b := a.pack[f.PackOffset : f.PackOffset+f.FileLength]
		if Sum(b) != f.FileHash {
			return nil, fmt.Errorf("%w: %s", ErrFileHash, f.Path)
		}
		files[f.Path] = b
	}
	return files, nil
}
