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
	files, _, err := ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	return Build(files, chunkSize)
}

// ReadDir returns the files of the app directory dir and of its Include
// list, with a MetaFile whose Include is empty, ready for Build. A caller
// can change the files first, as cxmanifest does for the ir and forth forms.
func ReadDir(dir string) (fstest.MapFS, Meta, error) {
	var meta Meta
	b, err := os.ReadFile(filepath.Join(dir, MetaFile))
	if err == nil {
		err = json.Unmarshal(b, &meta)
	}
	if err != nil {
		return nil, meta, fmt.Errorf("%s: %w", MetaFile, err)
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
		return nil, meta, err
	}
	for _, inc := range meta.Include {
		src := filepath.Join(dir, filepath.FromSlash(inc))
		st, err := os.Stat(src)
		if err != nil {
			return nil, meta, fmt.Errorf("%s: include: %w", dir, err)
		}
		if st.IsDir() {
			err = addTree(src, "")
		} else {
			err = add(filepath.Base(src), src)
		}
		if err != nil {
			return nil, meta, err
		}
	}
	meta.Include = nil
	return files, meta, SetMeta(files, meta)
}

// SetMeta writes meta to the MetaFile of files.
func SetMeta(files fstest.MapFS, meta Meta) error {
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	files[MetaFile] = &fstest.MapFile{Data: b}
	return nil
}
