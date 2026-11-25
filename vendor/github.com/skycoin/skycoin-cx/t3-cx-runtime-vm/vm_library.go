package vm

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// LibraryDispatchVmFunction runs the vm form of function of one system
// library and reports false for a number without one. Its inputs and then
// its outputs lie in memory from block, at the offsets of its native
// function. A vm form reads only scalars, so the vm runs it without a heap.
// The switches are generated from constants.LibraryFunctionDefinitionTable
// (decision D96).
type LibraryDispatchVmFunction func(function uint32, memory []byte, block uint32) bool

// LibraryDispatchesVm holds the vm switch of each system library this
// binary links, by library number, nil for a library it leaves out. The
// lowering emits ir.OpcodeLibraryCall only for the functions
// LibraryFunctionVmLinked reports.
// VIOLATION: state outside State. Owner: t3-cx-runtime-vm; moves to CXNativeFunctionMng in phase 1.
var LibraryDispatchesVm [constants.LibraryCount]LibraryDispatchVmFunction

// LibraryFunctionVmLinked reports whether function of library has a vm form
// and this binary links its library.
func LibraryFunctionVmLinked(library int, function int) bool {
	return library > 0 && library < constants.LibraryCount && LibraryDispatchesVm[library] != nil && constants.LibraryFunctionSignatureOf(library, function).LibraryFunctionSignatureVm
}

// LibraryStrView returns the bytes of the str whose pointer is at address,
// as the vm form of a library function reads a str input, and false for a
// str outside memory.
func LibraryStrView(memory []byte, address uint32) ([]byte, bool) {
	return memoryStrRead(memory, address, uint32(len(memory)))
}
