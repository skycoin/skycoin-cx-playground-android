// Package programstore keeps fetched programs on the SD card, or in the
// app's files dir on a phone, and in RAM. See "Loader, lister and runner
// (#17)" in docs/ProgramDistribution.md.
//
// The store directory holds:
//
//	list.bin            the last verified manifest list
//	m/<address>.bin     one verified manifest per program
//	p/<address>.pack    the program's linear packing, written block by block
//	                    as blocks are verified, so a stopped download resumes
//	index.txt           one line per program: number, address, state,
//	                    length and name, which the lister reads alone
//	trusted.txt         the trusted signer keys, one per line (D27)
//
// A medium or super programnetwork node also keeps subscriptions.txt and
// subscribers.txt there (#34).
package programstore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// The states of a program in index.txt.
const (
	StateRemote  = "remote"  // the manifest is here, no block yet
	StatePartial = "partial" // some blocks are here
	StateCached  = "cached"  // every block is here and verified
)

// Store is a store directory.
type Store struct {
	Dir string
}

// Open returns the store in dir, making its folders.
func Open(dir string) (*Store, error) {
	for _, sub := range []string{"m", "p"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{Dir: dir}, nil
}

func (s *Store) manifestPath(addr programblocks.Hash) string {
	return filepath.Join(s.Dir, "m", addr.String()+".bin")
}

func (s *Store) packingPath(addr programblocks.Hash) string {
	return filepath.Join(s.Dir, "p", addr.String()+".pack")
}

// IndexPath is the path of index.txt.
func (s *Store) IndexPath() string { return filepath.Join(s.Dir, "index.txt") }

// TrustedPath is the path of trusted.txt.
func (s *Store) TrustedPath() string { return filepath.Join(s.Dir, "trusted.txt") }

// writeFileSynced writes b to path through a temporary file, synced and
// renamed, so a reader sees the old file or the new one.
func writeFileSynced(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SaveManifest verifies an encoded manifest, which must be signed by one of
// trusted, and keeps it under its address.
func (s *Store) SaveManifest(encoded []byte, trusted []cipher.PubKey) (*programblocks.Manifest, programblocks.Hash, error) {
	m, addr, err := decodeTrusted(encoded, programblocks.Hash{}, trusted)
	if err != nil {
		return nil, addr, err
	}
	return m, addr, writeFileSynced(s.manifestPath(addr), encoded)
}

func decodeTrusted(encoded []byte, want programblocks.Hash, trusted []cipher.PubKey) (*programblocks.Manifest, programblocks.Hash, error) {
	m, addr, err := programblocks.Decode(encoded, want)
	if err != nil {
		return nil, addr, err
	}
	for _, k := range trusted {
		if k == m.SignerPublicKey {
			return m, addr, nil
		}
	}
	return nil, addr, fmt.Errorf("%w: %s", programblocks.ErrUntrusted, m.SignerPublicKey.Hex())
}

// Manifest reads and verifies the manifest kept under addr.
func (s *Store) Manifest(addr programblocks.Hash) (*programblocks.Manifest, error) {
	b, err := os.ReadFile(s.manifestPath(addr))
	if err != nil {
		return nil, err
	}
	m, _, err := programblocks.Decode(b, addr)
	return m, err
}

// SaveList keeps a verified manifest list, encoded, with its programs'
// manifests, which are the list's files keyed by program name. Every
// manifest must be signed by one of trusted. It rewrites index.txt with the
// programs of the list in name order, keeping the state of the ones whose
// packing is here.
func (s *Store) SaveList(encoded []byte, programs map[string][]byte, trusted []cipher.PubKey) ([]Row, error) {
	if _, _, err := decodeTrusted(encoded, programblocks.Hash{}, trusted); err != nil {
		return nil, fmt.Errorf("manifest list: %w", err)
	}
	names := make([]string, 0, len(programs))
	for name := range programs {
		names = append(names, name)
	}
	sort.Strings(names)
	rows := make([]Row, 0, len(names))
	for i, name := range names {
		m, addr, err := s.SaveManifest(programs[name], trusted)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		state, err := s.packingState(addr, m)
		if err != nil {
			return nil, err
		}
		rows = append(rows, Row{Number: i + 1, Address: addr.String(), State: state, Length: int64(m.PackLength), Name: name})
	}
	if err := writeFileSynced(filepath.Join(s.Dir, "list.bin"), encoded); err != nil {
		return nil, err
	}
	return rows, s.WriteIndex(rows)
}

// List reads and verifies the kept manifest list.
func (s *Store) List() (*programblocks.Manifest, programblocks.Hash, error) {
	b, err := os.ReadFile(filepath.Join(s.Dir, "list.bin"))
	if err != nil {
		return nil, programblocks.Hash{}, err
	}
	return programblocks.Decode(b, programblocks.Hash{})
}

// packingState returns the state of the packing of m from the blocks on
// disk.
func (s *Store) packingState(addr programblocks.Hash, m *programblocks.Manifest) (string, error) {
	p, err := s.OpenPacking(addr, m)
	if errors.Is(err, fs.ErrNotExist) {
		return StateRemote, nil
	}
	if err != nil {
		return "", err
	}
	defer p.Close()
	return p.State(), nil
}

// Packing is the linear packing of one program on disk, with the
// assembly that tells which blocks are in.
type Packing struct {
	Address  programblocks.Hash
	Manifest *programblocks.Manifest
	Assembly *programblocks.Assembly
	file     *os.File
	// writes counts the block writes since the last sync.
	writes int
}

// syncEvery is the number of block writes between two syncs. A crash loses
// at most these blocks, and the next open finds them missing.
const syncEvery = 32

// OpenPacking opens the packing of m, the manifest at addr, without creating
// it, and reads back every block whose hash matches, so a download resumes
// where it stopped.
func (s *Store) OpenPacking(addr programblocks.Hash, m *programblocks.Manifest) (*Packing, error) {
	return s.openPacking(addr, m, os.O_RDWR)
}

// CreatePacking opens the packing of m like OpenPacking, creating an
// empty one when there is none.
func (s *Store) CreatePacking(addr programblocks.Hash, m *programblocks.Manifest) (*Packing, error) {
	return s.openPacking(addr, m, os.O_RDWR|os.O_CREATE)
}

func (s *Store) openPacking(addr programblocks.Hash, m *programblocks.Manifest, flag int) (*Packing, error) {
	f, err := os.OpenFile(s.packingPath(addr), flag, 0o600)
	if err != nil {
		return nil, err
	}
	p := &Packing{Address: addr, Manifest: m, Assembly: programblocks.NewAssembly(m), file: f}
	if err := p.readBack(); err != nil {
		f.Close()
		return nil, err
	}
	p.Assembly.BlockStored = p.writeBlock
	return p, nil
}

// readBack feeds the blocks on disk to the assembly, which keeps the ones
// whose hash matches. A block never written reads as zeros and fails.
func (p *Packing) readBack() error {
	st, err := p.file.Stat()
	if err != nil {
		return err
	}
	size := uint64(st.Size())
	buf := make([]byte, programblocks.DefaultBlockSize)
	for _, e := range p.Manifest.Packing {
		end := e.PackOffset + uint64(e.BlockLength)
		if end > size {
			continue
		}
		if int(e.BlockLength) > cap(buf) {
			buf = make([]byte, e.BlockLength)
		}
		b := buf[:e.BlockLength]
		if _, err := p.file.ReadAt(b, int64(e.PackOffset)); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if _, err := p.Assembly.Accept(e.PackOffset, b); err != nil && !errors.Is(err, programblocks.ErrBlockHash) {
			return err
		}
	}
	return nil
}

func (p *Packing) writeBlock(offset uint64, block []byte) error {
	if _, err := p.file.WriteAt(block, int64(offset)); err != nil {
		return err
	}
	p.writes++
	if p.writes >= syncEvery {
		p.writes = 0
		return p.file.Sync()
	}
	return nil
}

// State is StateCached when every block is in, StatePartial when some are
// and StateRemote when none are.
func (p *Packing) State() string {
	switch p.Assembly.MissingBlocks() {
	case 0:
		return StateCached
	case len(p.Manifest.Blocks):
		return StateRemote
	}
	return StatePartial
}

// Close syncs and closes the packing file.
func (p *Packing) Close() error {
	err := p.file.Sync()
	if cerr := p.file.Close(); err == nil {
		err = cerr
	}
	return err
}

// Download fetches the blocks the packing of the program at addr misses
// through c and keeps each one as it is verified, then returns the
// program's verified files. A stopped download resumes from the blocks on
// disk.
func (s *Store) Download(c *programblocks.Client, addr programblocks.Hash) (map[string][]byte, error) {
	m, err := s.Manifest(addr)
	if err != nil {
		return nil, err
	}
	p, err := s.CreatePacking(addr, m)
	if err != nil {
		return nil, err
	}
	fetchErr := c.FetchInto(addr, p.Assembly)
	state := p.State()
	if err := p.Close(); err != nil && fetchErr == nil {
		fetchErr = err
	}
	if err := s.SetState(addr, state); err != nil && fetchErr == nil {
		fetchErr = err
	}
	if fetchErr != nil {
		return nil, fetchErr
	}
	return p.Assembly.Files()
}

// Files reads the program at addr from disk and returns its files after
// checking every block and file.
func (s *Store) Files(addr programblocks.Hash) (map[string][]byte, error) {
	m, err := s.Manifest(addr)
	if err != nil {
		return nil, err
	}
	p, err := s.OpenPacking(addr, m)
	if err != nil {
		return nil, err
	}
	defer p.Close()
	return p.Assembly.Files()
}

// Clear removes the packing of the program at addr, keeping its manifest,
// so the program is remote again.
func (s *Store) Clear(addr programblocks.Hash) error {
	if err := os.Remove(s.packingPath(addr)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return s.SetState(addr, StateRemote)
}
