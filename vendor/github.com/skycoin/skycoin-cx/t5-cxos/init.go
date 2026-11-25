//go:build !nocxos

package cxos

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only LibraryLink; registration API, keep
)

// RegisterPackage links the file and process libraries.
func RegisterPackage(prgrm *ast.CXProgram) {
	LibraryFileRegister(prgrm)
	LibraryProcessRegister(prgrm)
}

// LibraryFileRegister links the file library, the file functions of sys/os
// and the CPU profile.
func LibraryFileRegister(prgrm *ast.CXProgram) {
	opcodes.LibraryLink(constants.LibraryNumberFile, LibraryFileDispatch, LibraryFileDispatchVm)
}

// LibraryProcessRegister links the process library, os.Run.
func LibraryProcessRegister(prgrm *ast.CXProgram) {
	opcodes.LibraryLink(constants.LibraryNumberProcess, LibraryProcessDispatch, LibraryProcessDispatchVm)
}
