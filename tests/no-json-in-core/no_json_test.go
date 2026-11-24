// Package nojsonincore fails when a Go file outside microagents/ and
// vendor/ imports encoding/json (#38: "Json manifest banned. No json in
// core"). The micro agents keep JSON for the Anthropic API transcripts the
// API defines.
package nojsonincore

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestNoEncodingJSONImports(t *testing.T) {
	root := "../.."
	skip := map[string]bool{"vendor": true, "microagents": true, "build": true, ".git": true}
	files := 0
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if skip[filepath.ToSlash(rel)] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		files++
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			if path, _ := strconv.Unquote(imp.Path.Value); path == "encoding/json" {
				t.Errorf("%s imports encoding/json", filepath.ToSlash(rel))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 100 {
		t.Fatalf("checked only %d Go files", files)
	}
	t.Logf("%d Go files checked", files)
}
