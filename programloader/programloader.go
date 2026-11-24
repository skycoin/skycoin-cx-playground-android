// Package programloader runs the program loader, loader.cx, a CX program
// that lists, downloads and verifies a program with the net and cipher
// libraries (#39), and takes the files it hands over with app.ProgramFile.
package programloader

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
)

// Kind is the app kind of a loader app. Its files hold loader.cx, by
// [include], and its args are the loader's.
const Kind = "loader"

// Entry is the file name of the loader program.
const Entry = "loader.cx"

// Source is loader.cx.
//
//go:embed loader.cx
var Source []byte

// Libraries are the system libraries a loader run enables.
var Libraries = []string{"net", "cipher"}

// Load runs the loader program, entry in files, with args: the server, the
// program name and settings. It returns the files the loader handed over.
// opts may set NetRecord or NetReplay.
func Load(files map[string][]byte, entry string, args []string, opts cxrun.Options, out io.Writer) (map[string][]byte, error) {
	handed := map[string][]byte{}
	var repeated []string
	opts.Args, opts.Libraries = args, Libraries
	opts.ProgramFile = func(path string, data []byte) bool {
		if _, ok := handed[path]; ok {
			repeated = append(repeated, path)
			return false
		}
		handed[path] = bytes.Clone(data)
		return true
	}
	if err := cxrun.RunWith(files, entry, opts, nil, out); err != nil {
		return nil, fmt.Errorf("loader: %w", err)
	}
	if len(repeated) > 0 {
		return nil, fmt.Errorf("loader handed %q twice", repeated)
	}
	if len(handed) == 0 {
		return nil, fmt.Errorf("loader handed no files")
	}
	return handed, nil
}
