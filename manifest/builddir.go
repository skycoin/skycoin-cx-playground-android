package manifest

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing/fstest"
)

// BuildDir is Build for the app directory dir. It also adds the files that
// the Include list of its MetaFile names, so an app can use sources kept
// elsewhere in the repository without a copy.
func BuildDir(dir string, chunkSize int) (*Manifest, map[Hash][]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, MetaFile))
	if err != nil {
		return Build(os.DirFS(dir), chunkSize)
	}
	var meta Meta
	if err := json.Unmarshal(b, &meta); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", MetaFile, err)
	}
	if len(meta.Include) == 0 {
		return Build(os.DirFS(dir), chunkSize)
	}
	files := fstest.MapFS{}
	add := func(name, src string) error {
		if _, ok := files[name]; ok {
			return fmt.Errorf("%s: %s is listed twice", dir, name)
		}
		data, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		files[name] = &fstest.MapFile{Data: data}
		return nil
	}
	addTree := func(root string, prefix string) error {
		return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.Type().IsRegular() {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil || rel == MetaFile {
				return err
			}
			return add(filepath.ToSlash(filepath.Join(prefix, rel)), p)
		})
	}
	if err := addTree(dir, ""); err != nil {
		return nil, nil, err
	}
	for _, inc := range meta.Include {
		src := filepath.Join(dir, filepath.FromSlash(inc))
		st, err := os.Stat(src)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: include: %w", dir, err)
		}
		if st.IsDir() {
			err = addTree(src, "")
		} else {
			err = add(filepath.Base(src), src)
		}
		if err != nil {
			return nil, nil, err
		}
	}
	meta.Include = nil
	if files[MetaFile] == nil {
		b, err := json.Marshal(meta)
		if err != nil {
			return nil, nil, err
		}
		files[MetaFile] = &fstest.MapFile{Data: b}
	}
	return Build(files, chunkSize)
}
