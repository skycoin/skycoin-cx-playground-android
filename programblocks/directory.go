package programblocks

import (
	"testing/fstest"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
)

// ReadProgramDirectory returns the files of the app directory dir, keyed by
// path, with cxapp.json and its Include list resolved as manifest.ReadDir
// does. cxapp.json stays a file of the program, so a client can rebuild the
// app's manifest.Manifest from what it downloads.
func ReadProgramDirectory(dir string) (map[string][]byte, error) {
	fsys, _, err := manifest.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(fsys))
	for p, f := range fsys {
		files[p] = f.Data
	}
	return files, nil
}

// AppManifest returns the app manifest of downloaded program files, built
// from their cxapp.json, ready for apprun.Run.
func AppManifest(files map[string][]byte) (*manifest.Manifest, error) {
	fsys := fstest.MapFS{}
	for p, b := range files {
		fsys[p] = &fstest.MapFile{Data: b}
	}
	m, _, err := manifest.Build(fsys, 0)
	return m, err
}
