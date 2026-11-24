package manifest

import (
	"fmt"

	"github.com/skycoin/skycoin-cx-playground-android/textform"
)

// The keys of the [app] section of MetaFile.
var metaKeys = []string{"name", "description", "kind", "form", "entry", "cxir_example"}

// ParseMeta reads MetaFile in the text form (#38): an [app] section of
// key = value lines, and optional [args] and [include] sections with one
// entry per line.
func ParseMeta(b []byte) (Meta, error) {
	var m Meta
	d, err := textform.Parse(string(b))
	if err != nil {
		return m, fmt.Errorf("%s: %w", MetaFile, err)
	}
	if err := d.Only("app", "args", "include"); err != nil {
		return m, fmt.Errorf("%s: %w", MetaFile, err)
	}
	s, ok := d.Section("app")
	if !ok {
		return m, fmt.Errorf("%s: no [app] section", MetaFile)
	}
	k, err := s.Keys(metaKeys...)
	if err != nil {
		return m, fmt.Errorf("%s: %w", MetaFile, err)
	}
	m.Name, m.Description, m.Kind, m.Form, m.Entry, m.CXIRExample = k["name"], k["description"], k["kind"], k["form"], k["entry"], k["cxir_example"]
	if s, ok := d.Section("args"); ok {
		m.Args = s.Lines
	}
	if s, ok := d.Section("include"); ok {
		m.Include = s.Lines
	}
	return m, nil
}

// Encode returns m in the text form of MetaFile.
func (m Meta) Encode() []byte {
	var w textform.Writer
	w.Comment("CX app, text form.")
	w.Section("app")
	for _, kv := range [][2]string{{"name", m.Name}, {"description", m.Description}, {"kind", m.Kind}, {"form", m.Form}, {"entry", m.Entry}, {"cxir_example", m.CXIRExample}} {
		if kv[1] != "" {
			w.Key(kv[0], kv[1])
		}
	}
	for _, list := range []struct {
		name  string
		lines []string
	}{{"args", m.Args}, {"include", m.Include}} {
		if len(list.lines) == 0 {
			continue
		}
		w.Section(list.name)
		for _, l := range list.lines {
			w.Line(l)
		}
	}
	return w.Bytes()
}
