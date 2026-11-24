package vm

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// LibraryFunctionVm is the vm form of a system library function. Its inputs
// and then its outputs lie in memory from block, at the offsets of its native
// function. It reads only scalars, so the vm runs it without a heap.
type LibraryFunctionVm func(memory []byte, block uint32)

// LibraryFunctionVmHandlers holds the vm form of each system library
// function that has one, by library number and then function number. The
// lowering emits ir.OpcodeLibraryCall only for these.
// VIOLATION: state outside State. Owner: t3-cx-runtime-vm; moves to CXNativeFunctionMng in phase 1.
var LibraryFunctionVmHandlers [constants.LibraryCount][]LibraryFunctionVm

// LibraryFunctionVmHandler returns the vm form of function of library, nil
// when it has none.
func LibraryFunctionVmHandler(library int, function int) LibraryFunctionVm {
	if library <= 0 || library >= constants.LibraryCount || function <= 0 || function >= len(LibraryFunctionVmHandlers[library]) {
		return nil
	}
	return LibraryFunctionVmHandlers[library][function]
}
