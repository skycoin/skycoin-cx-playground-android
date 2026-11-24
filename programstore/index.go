package programstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// Row is one program of index.txt.
type Row struct {
	Number  int
	Address string // the manifest address in hex
	State   string // StateRemote, StatePartial or StateCached
	Length  int64  // the length of the linear packing in bytes
	Name    string
}

// Line is the row as one line of index.txt, without the newline. The name
// comes last, so it may hold spaces.
func (r Row) Line() string {
	return fmt.Sprintf("%d %s %s %d %s", r.Number, r.Address, r.State, r.Length, r.Name)
}

// WriteIndex replaces index.txt with rows.
func (s *Store) WriteIndex(rows []Row) error {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.Line())
		b.WriteByte('\n')
	}
	return writeFileSynced(s.IndexPath(), []byte(b.String()))
}

// ReadIndex is the lister: it reads index.txt with one read and returns its
// rows. The strings of every row share the one copy of the file, so the
// allocations do not grow with the number of rows beyond the row slice. A
// store without an index has no rows.
func (s *Store) ReadIndex() ([]Row, error) {
	b, err := os.ReadFile(s.IndexPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseIndex(string(b))
}

// ParseIndex parses the text of index.txt.
func ParseIndex(text string) ([]Row, error) {
	rows := make([]Row, 0, strings.Count(text, "\n")+1)
	for n := 1; len(text) > 0; n++ {
		line := text
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			line, text = text[:i], text[i+1:]
		} else {
			text = ""
		}
		if line == "" {
			continue
		}
		var fields [4]string
		rest := line
		for f := range fields {
			i := strings.IndexByte(rest, ' ')
			if i < 0 {
				return nil, fmt.Errorf("index.txt line %d: want number, address, state, length and name", n)
			}
			fields[f], rest = rest[:i], rest[i+1:]
		}
		number, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("index.txt line %d: number: %v", n, err)
		}
		length, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("index.txt line %d: length: %v", n, err)
		}
		rows = append(rows, Row{Number: number, Address: fields[1], State: fields[2], Length: length, Name: rest})
	}
	return rows, nil
}

// SetState sets the state of the program at addr in index.txt.
func (s *Store) SetState(addr programblocks.Hash, state string) error {
	rows, err := s.ReadIndex()
	if err != nil {
		return err
	}
	hex := addr.String()
	for i := range rows {
		if rows[i].Address == hex {
			rows[i].State = state
			return s.WriteIndex(rows)
		}
	}
	return fmt.Errorf("program %s is not in index.txt", hex)
}
