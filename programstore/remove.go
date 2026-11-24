package programstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// FindRow returns the row of index.txt whose address starts with prefix,
// hex of any case. No match or more than one is an error.
func (s *Store) FindRow(prefix string) (Row, error) {
	rows, err := s.ReadIndex()
	if err != nil {
		return Row{}, err
	}
	prefix = strings.ToLower(prefix)
	var found []Row
	for _, r := range rows {
		if strings.HasPrefix(r.Address, prefix) {
			found = append(found, r)
		}
	}
	switch len(found) {
	case 0:
		return Row{}, fmt.Errorf("no program %s in %s", prefix, s.IndexPath())
	case 1:
		return found[0], nil
	}
	names := make([]string, len(found))
	for i, r := range found {
		names[i] = r.Address[:16] + " " + r.Name
	}
	return Row{}, fmt.Errorf("prefix %s matches %d programs: %s", prefix, len(found), strings.Join(names, ", "))
}

// Remove deletes the manifest, the packing and the index row of the
// program at addr, and numbers the remaining rows again.
func (s *Store) Remove(addr programblocks.Hash) error {
	for _, path := range []string{s.packingPath(addr), s.manifestPath(addr)} {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	rows, err := s.ReadIndex()
	if err != nil {
		return err
	}
	kept := rows[:0]
	for _, r := range rows {
		if r.Address != addr.String() {
			r.Number = len(kept) + 1
			kept = append(kept, r)
		}
	}
	return s.WriteIndex(kept)
}

// ManifestBytes is the size of the manifest kept under addr, 0 when there
// is none.
func (s *Store) ManifestBytes(addr programblocks.Hash) int64 {
	st, err := os.Stat(s.manifestPath(addr))
	if err != nil {
		return 0
	}
	return st.Size()
}

// WriteLines replaces the file at path with lines, one per line, through a
// synced temporary file.
func WriteLines(path string, lines []string) error {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return writeFileSynced(path, []byte(b.String()))
}

// ReadLines returns the lines of the file at path that are not empty and
// do not start with #, trimmed. A missing file has no lines.
func ReadLines(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
	}
	return out, nil
}
