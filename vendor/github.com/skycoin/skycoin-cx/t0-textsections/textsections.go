// Package textsections reads and writes the CX text form of a file of
// sections. A section is a header line in brackets naming the section and
// its columns, `[name] column column`, then one entry per line, and a blank
// line before the next header. Lines starting with # are comments. The
// execution records (docs/ExecutionRecords.md) and the package loader store
// their data in this form.
package textsections

import (
	"bytes"
	"fmt"
	"strings"
)

// TextSection is one section of a file.
type TextSection struct {
	TextSectionName    string
	TextSectionColumns []string
	TextSectionEntries []TextSectionEntry
}

// TextSectionEntry is one entry line of a section.
type TextSectionEntry struct {
	// TextSectionEntryLine is the line number in the file, from 1.
	TextSectionEntryLine int
	TextSectionEntryText string
}

// TextSectionsRead returns the sections of content in file order. A header
// without a blank line before it or an entry outside a section is an error.
func TextSectionsRead(content []byte) ([]TextSection, error) {
	var sections []TextSection
	inSection := false
	for index, line := range strings.Split(string(content), "\n") {
		switch {
		case line == "":
			inSection = false
		case strings.HasPrefix(line, "#"):
		case strings.HasPrefix(line, "["):
			if inSection {
				return nil, fmt.Errorf("line %d: a header without a blank line before it: %q", index+1, line)
			}
			name, columns, found := strings.Cut(line[1:], "]")
			if !found {
				return nil, fmt.Errorf("line %d: a header without ]: %q", index+1, line)
			}
			sections = append(sections, TextSection{TextSectionName: name, TextSectionColumns: strings.Fields(columns)})
			inSection = true
		default:
			if !inSection {
				return nil, fmt.Errorf("line %d: an entry outside a section: %q", index+1, line)
			}
			last := &sections[len(sections)-1]
			last.TextSectionEntries = append(last.TextSectionEntries, TextSectionEntry{TextSectionEntryLine: index + 1, TextSectionEntryText: line})
		}
	}
	return sections, nil
}

// TextSectionsWrite returns the comment lines, each after "# ", and then the
// sections, a blank line between two of them.
func TextSectionsWrite(comments []string, sections []TextSection) []byte {
	var out bytes.Buffer
	for _, comment := range comments {
		out.WriteString("# " + comment + "\n")
	}
	for index, section := range sections {
		if index > 0 {
			out.WriteString("\n")
		}
		out.WriteString("[" + section.TextSectionName + "]")
		for _, column := range section.TextSectionColumns {
			out.WriteString(" " + column)
		}
		out.WriteString("\n")
		for _, entry := range section.TextSectionEntries {
			out.WriteString(entry.TextSectionEntryText + "\n")
		}
	}
	return out.Bytes()
}

// TextSectionFind returns the first section named name.
func TextSectionFind(sections []TextSection, name string) (TextSection, bool) {
	for _, section := range sections {
		if section.TextSectionName == name {
			return section, true
		}
	}
	return TextSection{}, false
}
