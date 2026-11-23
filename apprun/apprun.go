// Package apprun runs the program of an app in its form: CX source through
// the compiler, a serialized CX program without one, or Forth on cx-forth.
package apprun

import (
	"fmt"
	"io"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/forthrun"
	"github.com/skycoin/skycoin-cx-playground-android/manifest"
)

// Run runs the app of m, whose verified files are files.
func Run(m *manifest.Manifest, files map[string][]byte, opts cxrun.Options, h cxrun.Host, out io.Writer) error {
	switch m.Form {
	case manifest.FormSource:
		return cxrun.RunWith(files, m.Entry, opts, h, out)
	case manifest.FormIR:
		_, err := cxrun.RunIRWith(files[m.Entry], cxrun.ExecutorAuto, opts, h, out)
		return err
	case manifest.FormForth:
		// Only a gfx app has a canvas for the grid and keys.
		if m.Kind != "gfx" {
			h = nil
		}
		return forthrun.Run(string(files[m.Entry]), h, out)
	}
	return fmt.Errorf("unknown form %q", m.Form)
}
