// Package textform reads and writes the CX text form that replaces JSON
// (#38, D24): a "[name]" header line per section, one entry per line, a
// blank line before the next header, and ";" starting a comment line. A
// key section holds "key = value" lines, a table section comma separated
// columns named by a comment line, the last column taking the rest of the
// line. See "Text form" in docs/ProgramDistribution.md.
package textform

import (
	"fmt"
	"strings"
)

// Section is one section: its name and its entry lines, comments removed.
type Section struct {
	Name  string
	Lines []string
	// Line is the line number of each entry, for errors.
	Line []int
}

// Document is the sections of a text in order.
type Document struct {
	Sections []Section
}

// Parse splits text into sections. A line before the first header, other
// than a comment or a blank line, is an error, and so is a section named
// twice.
func Parse(text string) (*Document, error) {
	d := &Document{}
	seen := map[string]bool{}
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, ";"):
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && !strings.ContainsAny(line, ",="):
			name := line[1 : len(line)-1]
			if seen[name] {
				return nil, fmt.Errorf("line %d: section [%s] twice", n+1, name)
			}
			seen[name] = true
			d.Sections = append(d.Sections, Section{Name: name})
		case len(d.Sections) == 0:
			return nil, fmt.Errorf("line %d: entry before the first section header", n+1)
		default:
			s := &d.Sections[len(d.Sections)-1]
			s.Lines, s.Line = append(s.Lines, line), append(s.Line, n+1)
		}
	}
	return d, nil
}

// Section returns the section called name.
func (d *Document) Section(name string) (Section, bool) {
	for _, s := range d.Sections {
		if s.Name == name {
			return s, true
		}
	}
	return Section{}, false
}

// Only fails when d has a section whose name is not in names.
func (d *Document) Only(names ...string) error {
	for _, s := range d.Sections {
		known := false
		for _, n := range names {
			known = known || n == s.Name
		}
		if !known {
			return fmt.Errorf("unknown section [%s]", s.Name)
		}
	}
	return nil
}

// Keys reads a key section into a map. Each line is "key = value"; a key
// given twice, or one not in allowed, is an error.
func (s Section) Keys(allowed ...string) (map[string]string, error) {
	out := map[string]string{}
	for i, l := range s.Lines {
		k, v, ok := strings.Cut(l, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if !ok || k == "" {
			return nil, fmt.Errorf("line %d: want key = value in [%s]", s.Line[i], s.Name)
		}
		known := false
		for _, a := range allowed {
			known = known || a == k
		}
		if !known {
			return nil, fmt.Errorf("line %d: unknown key %q in [%s]", s.Line[i], k, s.Name)
		}
		if _, dup := out[k]; dup {
			return nil, fmt.Errorf("line %d: key %q twice in [%s]", s.Line[i], k, s.Name)
		}
		out[k] = v
	}
	return out, nil
}

// Rows reads a table section of columns columns. The last column takes
// the rest of the line, commas included.
func (s Section) Rows(columns int) ([][]string, error) {
	var out [][]string
	for i, l := range s.Lines {
		f := strings.SplitN(l, ",", columns)
		if len(f) != columns {
			return nil, fmt.Errorf("line %d: %d columns in [%s], want %d", s.Line[i], len(f), s.Name, columns)
		}
		for j := range f {
			f[j] = strings.TrimSpace(f[j])
		}
		out = append(out, f)
	}
	return out, nil
}

// Writer builds a text form. Sections after the first get a blank line
// before their header.
type Writer struct {
	b        strings.Builder
	sections int
}

// Comment writes a comment line.
func (w *Writer) Comment(text string) { w.b.WriteString("; " + text + "\n") }

// Section starts a section.
func (w *Writer) Section(name string) {
	if w.sections > 0 {
		w.b.WriteString("\n")
	}
	w.sections++
	w.b.WriteString("[" + name + "]\n")
}

// Key writes "key = value". A value must not hold a line break.
func (w *Writer) Key(key, value string) { w.b.WriteString(key + " = " + oneLine(value) + "\n") }

// Line writes one entry line as it is.
func (w *Writer) Line(text string) { w.b.WriteString(oneLine(text) + "\n") }

// Row writes comma separated columns.
func (w *Writer) Row(columns ...string) {
	for i := range columns {
		columns[i] = oneLine(columns[i])
	}
	w.b.WriteString(strings.Join(columns, ", ") + "\n")
}

// Bytes returns the text written so far.
func (w *Writer) Bytes() []byte { return []byte(w.b.String()) }

func oneLine(s string) string { return strings.NewReplacer("\r", " ", "\n", " ").Replace(s) }
