package main

import (
	"fmt"
	"path"
	"strings"
	"testing/fstest"

	"github.com/skycoin/skycoin-cx-forth/cxir"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/manifest"
)

// irEntry and forthEntry are the entry files of the ir and forth forms.
const (
	irEntry    = "main.cxir"
	forthEntry = "main.fs"
)

// buildApp is manifest.BuildDir with the program forms applied: an ir app's
// CX sources become one serialized program, and a forth app with a
// cxir_example gets that program's Forth translation.
func buildApp(dir string, chunk int) (*manifest.Manifest, map[manifest.Hash][]byte, error) {
	files, err := appFiles(dir)
	if err != nil {
		return nil, nil, err
	}
	return manifest.Build(files, chunk)
}

// appFiles returns the files of the app directory dir with the program
// forms applied and the final cxapp.json, the files the bundle holds.
func appFiles(dir string) (fstest.MapFS, error) {
	files, meta, err := manifest.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	switch meta.Form {
	case manifest.FormSource:
	case manifest.FormIR:
		err = serializeApp(files, &meta)
	case manifest.FormForth:
		err = forthApp(files, &meta)
	default:
		err = fmt.Errorf("unknown form %q", meta.Form)
	}
	if err == nil {
		err = manifest.SetMeta(files, meta)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", dir, err)
	}
	return files, nil
}

// serializeApp compiles the .cx files with the pinned skycoin-cx and replaces
// them with the serialized program.
func serializeApp(files fstest.MapFS, meta *manifest.Meta) error {
	entry := meta.Entry
	if entry == "" {
		entry = "main.cx"
	}
	srcs := map[string][]byte{}
	for name, f := range files {
		if path.Ext(name) == ".cx" {
			srcs[name] = f.Data
			delete(files, name)
		}
	}
	program, err := cxrun.Serialize(srcs, entry)
	if err != nil {
		return err
	}
	files[irEntry] = &fstest.MapFile{Data: program}
	meta.Entry = irEntry
	return nil
}

// forthApp writes the Forth translation of meta.CXIRExample to main.fs,
// followed by a call of its main.
func forthApp(files fstest.MapFS, meta *manifest.Meta) error {
	if meta.Entry == "" {
		meta.Entry = forthEntry
	}
	if meta.CXIRExample == "" {
		return nil
	}
	for _, ex := range cxir.Examples {
		if ex.Name != meta.CXIRExample {
			continue
		}
		src, err := cxir.Translate(&ex.Program)
		if err != nil {
			return err
		}
		files[meta.Entry] = &fstest.MapFile{Data: []byte(strings.TrimRight(src, "\n") + "\n" + cxir.WordName("main") + "\n")}
		meta.CXIRExample = ""
		return nil
	}
	return fmt.Errorf("no cxir example %q", meta.CXIRExample)
}
