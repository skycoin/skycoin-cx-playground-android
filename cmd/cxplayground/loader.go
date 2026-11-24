package main

import (
	"fmt"
	"io"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programloader"
)

// loaderProgram runs the CX loader that a loader app holds, with the app's
// args, and returns the program it handed over (#39).
func loaderProgram(m *manifest.Manifest, files map[string][]byte, out io.Writer) (*manifest.Manifest, map[string][]byte, error) {
	if len(m.Args) < 2 {
		return nil, nil, fmt.Errorf("a loader app takes the server and the program name as args")
	}
	handed, err := programloader.Load(files, m.Entry, m.Args, cxrun.Options{}, out)
	if err != nil {
		return nil, nil, err
	}
	app, err := programblocks.AppManifest(handed)
	if err != nil {
		return nil, nil, err
	}
	return app, handed, nil
}
