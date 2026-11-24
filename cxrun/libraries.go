package cxrun

import (
	"fmt"

	constants "github.com/skycoin/skycoin-cx/t1-constants"
)

// LibrariesAll names every system library of skycoin-cx: file, net and
// process, see constants.LibraryTable.
func LibrariesAll() []string {
	names := make([]string, 0, len(constants.LibraryTable))
	for _, library := range constants.LibraryTable {
		names = append(names, library.LibraryName)
	}
	return names
}

// librariesEnabled turns library names into the per library flags of
// CXProgram.ProgramLibrariesEnabled.
func librariesEnabled(names []string) (enabled [constants.LibraryCount]bool, err error) {
	for _, name := range names {
		library, found := constants.LibraryByName(name)
		if !found {
			return enabled, fmt.Errorf("unknown system library %q, the libraries are %v", name, LibrariesAll())
		}
		enabled[library.LibraryNumber] = true
	}
	return enabled, nil
}
