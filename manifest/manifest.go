// Package manifest describes a CX app: its metadata file, MetaFile, in the
// text form, and its files, each split into fixed size chunks addressed by
// their SHA-256, so every byte can be checked on arrival. The bundle and
// the network identify an app by its programblocks manifest address.
package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// Version is the manifest format version written by Build.
const Version = 1

// DefaultChunkSize is the chunk size used by Build.
const DefaultChunkSize = 64 << 10

// MetaFile is the app metadata file of a source directory, in the text form
// of ParseMeta. It is
// read by Build and is not itself listed in the manifest.
const MetaFile = "cxapp.txt"

// Hash is a hex encoded SHA-256 digest.
type Hash string

// Sum returns the hash of b.
func Sum(b []byte) Hash {
	s := sha256.Sum256(b)
	return Hash(hex.EncodeToString(s[:]))
}

// Valid reports whether h is 64 lower case hex digits.
func (h Hash) Valid() bool {
	if len(h) != 64 {
		return false
	}
	for _, c := range h {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// The program forms of an app. A FormSource app holds CX sources that the app
// compiles. A FormIR app holds one serialized CX program, the format of
// skycoin-cx t3-ast SerializeCXProgram that cx build appends to its core,
// which the app runs without a parser. A FormForth app holds Forth source for
// the cx-forth VM.
const (
	FormSource = ""
	FormIR     = "ir"
	FormForth  = "forth"
)

// Manifest lists the files of one app.
type Manifest struct {
	Version     int
	Name        string
	Description string
	Kind        string
	// Form is how the app holds its program: FormSource, FormIR or FormForth.
	Form      string
	Entry     string
	Args      []string
	ChunkSize int
	Files     []File
}

// File is one file of an app. Chunks holds the hash of each ChunkSize piece
// of the file in order, the last one possibly shorter.
type File struct {
	Path   string
	Size   int64
	Hash   Hash
	Chunks []Hash
}

// Meta is the content of MetaFile.
type Meta struct {
	Name        string
	Description string
	Kind        string
	Form        string
	Entry       string
	// Args is the program's os.Args, as the ++ arguments of cx.
	Args []string
	// Include lists files and directories outside the app directory, relative
	// to it, whose regular files BuildDir adds under their base names.
	Include []string
	// CXIRExample names a cx-forth cxir.Examples program. cxmanifest writes
	// its Forth translation to main.fs of a FormForth app.
	CXIRExample string
}

// Build makes the manifest for every regular file under fsys. It also
// returns the chunk contents keyed by hash, ready to be stored.
func Build(fsys fs.FS, chunkSize int) (*Manifest, map[Hash][]byte, error) {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	m := &Manifest{Version: Version, ChunkSize: chunkSize, Entry: "main.cx"}
	if b, err := fs.ReadFile(fsys, MetaFile); err == nil {
		meta, err := ParseMeta(b)
		if err != nil {
			return nil, nil, err
		}
		if len(meta.Include) > 0 {
			return nil, nil, errors.New(MetaFile + ": include needs BuildDir")
		}
		m.Name, m.Description, m.Kind, m.Form, m.Args = meta.Name, meta.Description, meta.Kind, meta.Form, meta.Args
		if meta.Entry != "" {
			m.Entry = meta.Entry
		}
	}
	chunks := map[Hash][]byte{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() || p == MetaFile {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		f := File{Path: p, Size: int64(len(b)), Hash: Sum(b), Chunks: []Hash{}}
		for off := 0; off < len(b); off += chunkSize {
			c := b[off:min(off+chunkSize, len(b))]
			h := Sum(c)
			f.Chunks = append(f.Chunks, h)
			chunks[h] = c
		}
		m.Files = append(m.Files, f)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	if m.Name == "" {
		return nil, nil, errors.New("app has no name, add " + MetaFile)
	}
	if err := m.Check(); err != nil {
		return nil, nil, err
	}
	return m, chunks, nil
}

// Check validates the structure of m: version, paths, chunk counts and
// hash syntax. It does not look at any content.
func (m *Manifest) Check() error {
	if m.Version != Version {
		return fmt.Errorf("manifest version %d, want %d", m.Version, Version)
	}
	if m.ChunkSize <= 0 {
		return errors.New("manifest chunk_size must be positive")
	}
	seen := map[string]bool{}
	entry := false
	for _, f := range m.Files {
		if !fs.ValidPath(f.Path) || f.Path == "." || path.Clean(f.Path) != f.Path || strings.Contains(f.Path, "\\") {
			return fmt.Errorf("bad file path %q", f.Path)
		}
		if seen[f.Path] {
			return fmt.Errorf("duplicate file path %q", f.Path)
		}
		seen[f.Path] = true
		entry = entry || f.Path == m.Entry
		if f.Size < 0 || !f.Hash.Valid() {
			return fmt.Errorf("%s: bad size or hash", f.Path)
		}
		n := (f.Size + int64(m.ChunkSize) - 1) / int64(m.ChunkSize)
		if int64(len(f.Chunks)) != n {
			return fmt.Errorf("%s: %d chunks, want %d", f.Path, len(f.Chunks), n)
		}
		for _, c := range f.Chunks {
			if !c.Valid() {
				return fmt.Errorf("%s: bad chunk hash %q", f.Path, c)
			}
		}
	}
	if !entry {
		return fmt.Errorf("entry %q is not in the file list", m.Entry)
	}
	return nil
}

// ChunkGetter returns the content of a chunk by hash.
type ChunkGetter func(Hash) ([]byte, error)

// Assemble fetches the chunks of f, checks each chunk, its size and the
// whole file hash, and returns the file content.
func (m *Manifest) Assemble(f File, get ChunkGetter) ([]byte, error) {
	var buf bytes.Buffer
	buf.Grow(int(f.Size))
	for i, h := range f.Chunks {
		c, err := get(h)
		if err != nil {
			return nil, fmt.Errorf("%s chunk %d: %w", f.Path, i, err)
		}
		if got := Sum(c); got != h {
			return nil, fmt.Errorf("%s chunk %d: hash %s, want %s", f.Path, i, got, h)
		}
		want := m.ChunkSize
		if i == len(f.Chunks)-1 {
			want = int(f.Size - int64(i)*int64(m.ChunkSize))
		}
		if len(c) != want {
			return nil, fmt.Errorf("%s chunk %d: %d bytes, want %d", f.Path, i, len(c), want)
		}
		buf.Write(c)
	}
	b := buf.Bytes()
	if int64(len(b)) != f.Size || Sum(b) != f.Hash {
		return nil, fmt.Errorf("%s: content does not match manifest", f.Path)
	}
	return b, nil
}

// ReadAllLimited reads r up to limit bytes and fails beyond it.
func ReadAllLimited(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("more than %d bytes", limit)
	}
	return b, nil
}
