package programblocks

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/skycoin/skycoin/src/cipher"
)

// The text form of a manifest, see "Text form" in
// docs/ProgramDistribution.md. It holds the same fields as the binary form
// and converts to it byte for byte. The address and signature are always
// taken over the binary form.

// TextComment starts a comment line.
const TextComment = ";"

// The sections of the text form, in order, with the column names of the
// list sections.
var (
	textHeaderKeys = []string{"magic", "version", "date", "manifest_length", "pack_length", "merkle_hash", "signer", "signature", "address"}
	textColumns    = map[string][]string{
		"arrays": {"kind", "count", "offset", "length", "hash"},
		"files":  {"path", "file_hash", "size", "offset", "block_size", "first_block", "block_count", "merkle_hash"},
		"hashes": {"block_hash", "block_id", "size", "offset"},
		"linear": {"block_id", "offset", "length"},
	}
	textSections = []string{"header", "arrays", "files", "hashes", "linear"}
)

// EncodeText returns the canonical text form of m.
func (m *Manifest) EncodeText() []byte {
	addr := m.Address() // also fills ManifestLength and Arrays
	var b bytes.Buffer
	b.WriteString(TextComment + " CX program manifest, text form. The address and signature are over the binary form.\n")
	b.WriteString("[header]\n")
	values := []string{Magic, strconv.Itoa(FormatVersion), u64(m.Date), u64(m.ManifestLength), u64(m.PackLength),
		m.MerkleHash.String(), hex.EncodeToString(m.SignerPublicKey[:]), hex.EncodeToString(m.Signature[:]), addr.String()}
	width := 0
	for _, k := range textHeaderKeys {
		width = max(width, len(k))
	}
	for i, k := range textHeaderKeys {
		fmt.Fprintf(&b, "%-*s = %s\n", width, k, values[i])
	}
	var rows [][]string
	for _, e := range m.Arrays {
		rows = append(rows, []string{u64(uint64(e.Kind)), u32(e.Count), u32(e.Offset), u32(e.Length), e.ArrayHash.String()})
	}
	writeTextList(&b, "arrays", rows)
	rows = rows[:0]
	for _, f := range m.Files {
		rows = append(rows, []string{quoteTextField(f.Path), f.FileHash.String(), u64(f.FileLength), u64(f.PackOffset), u32(f.BlockSize), u32(f.FirstBlockID), u32(f.BlockCount), f.MerkleHash.String()})
	}
	writeTextList(&b, "files", rows)
	rows = rows[:0]
	for _, e := range m.Blocks {
		rows = append(rows, []string{e.BlockHash.String(), u32(e.BlockID), u32(e.BlockLength), u64(e.FileOffset)})
	}
	writeTextList(&b, "hashes", rows)
	rows = rows[:0]
	for _, e := range m.Packing {
		rows = append(rows, []string{u32(e.BlockID), u64(e.PackOffset), u32(e.BlockLength)})
	}
	writeTextList(&b, "linear", rows)
	return b.Bytes()
}

func u64(v uint64) string { return strconv.FormatUint(v, 10) }
func u32(v uint32) string { return strconv.FormatUint(uint64(v), 10) }

// writeTextList writes a section with a comment line of column names and
// one row per line, each column padded to the widest value.
func writeTextList(b *bytes.Buffer, section string, rows [][]string) {
	cols := textColumns[section]
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	for _, r := range rows {
		for i, v := range r {
			widths[i] = max(widths[i], len(v))
		}
	}
	line := func(prefix string, r []string) {
		var s strings.Builder
		s.WriteString(prefix)
		for i, v := range r {
			if i == len(r)-1 {
				s.WriteString(v)
				break
			}
			s.WriteString(v)
			s.WriteString(",")
			s.WriteString(strings.Repeat(" ", widths[i]-len(v)+1))
		}
		b.WriteString(s.String() + "\n")
	}
	fmt.Fprintf(b, "\n[%s]\n", section)
	// The comment marker takes two columns, so the data rows start indented
	// by two spaces to line up with the names.
	line(TextComment+" ", cols)
	for _, r := range rows {
		line("  ", r)
	}
}

func quoteTextField(s string) string {
	if !strings.ContainsAny(s, ", \t\"") && !strings.HasPrefix(s, TextComment) && !strings.HasPrefix(s, "[") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// splitTextRow splits a row at commas outside double quotes and trims the
// padding around each field.
func splitTextRow(line string) ([]string, error) {
	var fields []string
	for {
		line = strings.TrimLeft(line, " \t")
		var field string
		if strings.HasPrefix(line, `"`) {
			var s strings.Builder
			i := 1
			for ; i < len(line); i++ {
				if line[i] != '"' {
					s.WriteByte(line[i])
					continue
				}
				if i+1 < len(line) && line[i+1] == '"' {
					s.WriteByte('"')
					i++
					continue
				}
				break
			}
			if i >= len(line) {
				return nil, fmt.Errorf("unterminated quote")
			}
			field, line = s.String(), strings.TrimLeft(line[i+1:], " \t")
			if line != "" && line[0] != ',' {
				return nil, fmt.Errorf("text after a quoted field")
			}
		} else {
			j := strings.IndexByte(line, ',')
			if j < 0 {
				j = len(line)
			}
			field, line = strings.TrimRight(line[:j], " \t"), line[j:]
		}
		fields = append(fields, field)
		if line == "" {
			return fields, nil
		}
		line = line[1:]
	}
}

// DecodeText parses the text form, rebuilds the binary form and checks it
// with Decode, so the address, signature and every hash are verified. The
// [arrays] section and the derived header fields must match what the binary
// form gives. It returns the manifest, its binary form and its address.
func DecodeText(text []byte) (*Manifest, []byte, Hash, error) {
	m := &Manifest{}
	header := map[string]string{}
	var arrays [][]string
	section := ""
	next := 0
	sc := bufio.NewScanner(bytes.NewReader(text))
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, TextComment) {
			continue
		}
		fail := func(format string, args ...any) error {
			return fmt.Errorf("%w: text line %d: %s", ErrMalformed, n, fmt.Sprintf(format, args...))
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			name := line[1 : len(line)-1]
			if next >= len(textSections) || textSections[next] != name {
				return nil, nil, Hash{}, fail("section [%s] out of order", name)
			}
			section = name
			next++
			continue
		}
		if section == "header" {
			k, v, ok := strings.Cut(line, "=")
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if !ok || header[k] != "" {
				return nil, nil, Hash{}, fail("want one key = value per header key")
			}
			header[k] = v
			continue
		}
		cols, ok := textColumns[section]
		if !ok || cols == nil {
			return nil, nil, Hash{}, fail("entry outside a section")
		}
		r, err := splitTextRow(line)
		if err == nil && len(r) != len(cols) {
			err = fmt.Errorf("%d fields, want %d", len(r), len(cols))
		}
		if err != nil {
			return nil, nil, Hash{}, fail("%v", err)
		}
		var p textFieldParser
		switch section {
		case "arrays":
			arrays = append(arrays, r)
		case "files":
			m.Files = append(m.Files, FileEntry{Path: r[0], FileHash: p.hash(r[1]), FileLength: p.u64(r[2]), PackOffset: p.u64(r[3]),
				BlockSize: p.u32(r[4]), FirstBlockID: p.u32(r[5]), BlockCount: p.u32(r[6]), MerkleHash: p.hash(r[7])})
		case "hashes":
			m.Blocks = append(m.Blocks, BlockEntry{BlockHash: p.hash(r[0]), BlockID: p.u32(r[1]), BlockLength: p.u32(r[2]), FileOffset: p.u64(r[3])})
		case "linear":
			m.Packing = append(m.Packing, BlockPacking{BlockID: p.u32(r[0]), PackOffset: p.u64(r[1]), BlockLength: p.u32(r[2])})
		}
		if p.err != nil {
			return nil, nil, Hash{}, fail("%v", p.err)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, nil, Hash{}, err
	}
	if next != len(textSections) {
		return nil, nil, Hash{}, fmt.Errorf("%w: text form has %d of %d sections", ErrTruncated, next, len(textSections))
	}
	for _, k := range textHeaderKeys {
		if header[k] == "" {
			return nil, nil, Hash{}, fmt.Errorf("%w: header has no %s", ErrMalformed, k)
		}
	}
	if len(header) != len(textHeaderKeys) {
		return nil, nil, Hash{}, fmt.Errorf("%w: unknown header key", ErrMalformed)
	}
	var p textFieldParser
	m.Date, m.PackLength, m.MerkleHash = p.u64(header["date"]), p.u64(header["pack_length"]), p.hash(header["merkle_hash"])
	copy(m.SignerPublicKey[:], p.bytes(header["signer"], len(cipher.PubKey{})))
	copy(m.Signature[:], p.bytes(header["signature"], len(cipher.Sig{})))
	if p.err != nil {
		return nil, nil, Hash{}, fmt.Errorf("%w: header: %v", ErrMalformed, p.err)
	}
	if header["magic"] != Magic || header["version"] != strconv.Itoa(FormatVersion) {
		return nil, nil, Hash{}, fmt.Errorf("%w: magic or version", ErrMalformed)
	}
	b := m.Encode()
	decoded, addr, err := Decode(b, Hash{})
	if err != nil {
		return nil, nil, addr, err
	}
	if header["manifest_length"] != u64(decoded.ManifestLength) || header["address"] != addr.String() {
		return nil, nil, addr, fmt.Errorf("%w: header manifest_length or address does not match the binary form", ErrMalformed)
	}
	if len(arrays) != len(decoded.Arrays) {
		return nil, nil, addr, fmt.Errorf("%w: [arrays] has %d rows, want %d", ErrMalformed, len(arrays), len(decoded.Arrays))
	}
	for i, e := range decoded.Arrays {
		want := []string{u64(uint64(e.Kind)), u32(e.Count), u32(e.Offset), u32(e.Length), e.ArrayHash.String()}
		if strings.Join(arrays[i], ",") != strings.Join(want, ",") {
			return nil, nil, addr, fmt.Errorf("%w: [arrays] row %d does not match the binary form", ErrArrayHash, i+1)
		}
	}
	return decoded, b, addr, nil
}

// textFieldParser parses fields and keeps the first error.
type textFieldParser struct{ err error }

func (p *textFieldParser) u64(s string) uint64 {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil && p.err == nil {
		p.err = fmt.Errorf("bad number %q", s)
	}
	return v
}

func (p *textFieldParser) u32(s string) uint32 {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil && p.err == nil {
		p.err = fmt.Errorf("bad number %q", s)
	}
	return uint32(v)
}

func (p *textFieldParser) bytes(s string, n int) []byte {
	b, err := hex.DecodeString(s)
	if (err != nil || len(b) != n) && p.err == nil {
		p.err = fmt.Errorf("want %d hex bytes, got %q", n, s)
	}
	return b
}

func (p *textFieldParser) hash(s string) Hash {
	var h Hash
	copy(h[:], p.bytes(s, len(h)))
	return h
}
