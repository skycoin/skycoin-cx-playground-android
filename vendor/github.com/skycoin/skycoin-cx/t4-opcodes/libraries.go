package opcodes

import (
	// GCREACH: t3-ast owns the heap and the GC. This file links library
	// switches and allocates nothing, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
)

// LibraryLink links system library into this binary. The old loop runs its
// functions through dispatch and the vm runs their vm forms through
// dispatchVm, the switches t2-cx-core-opcodegen writes from
// constants.LibraryFunctionDefinitionTable (decision D96). A call of one of
// its functions compiles to constants.OP_LIBRARY_CALL.
func LibraryLink(library int, dispatch ast.LibraryDispatchFunction, dispatchVm vm.LibraryDispatchVmFunction) {
	ast.LibraryDispatches[library] = dispatch
	vm.LibraryDispatchesVm[library] = dispatchVm
	ast.NativeNames.NativeNameIndexChanged()
}
