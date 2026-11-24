package init

import (
	"github.com/skycoin/skycoin-cx/t1-constants" // thin: only CALLSTACK_SIZE, NULL_HEAP_ADDRESS_OFFSET, STACK_SIZE; no depth gain
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t5-cipher"             // thin: only RegisterPackage; registration list, keep
	"github.com/skycoin/skycoin-cx/t5-cx-runtime-printer" // thin: only PrinterCompileErrorsRegister, PrinterOpcodesRegister; registration list, keep
	"github.com/skycoin/skycoin-cx/t5-cxos"               // thin: only RegisterPackage; registration list, keep
	cxoscore "github.com/skycoin/skycoin-cx/t5-cxos-core" // thin: only RegisterPackage; registration list, keep
	"github.com/skycoin/skycoin-cx/t5-tcp"                // thin: only RegisterPackage; registration list, keep
	"github.com/skycoin/skycoin-cx/t6-cxfx"               // thin: only RegisterPackage; registration list, keep

	// "github.com/skycoin/cx/cx/packages/http"

	"github.com/skycoin/skycoin-cx/t5-regexp" // thin: only RegisterPackage; registration list, keep
)

// RegisterPackages adds the native packages to prgrm and their functions to
// the global opcode tables.
func RegisterPackages(prgrm *ast.CXProgram) {
	cipher.RegisterPackage(prgrm)
	cxfx.RegisterPackage(prgrm)
	cxos.RegisterPackage(prgrm)
	cxoscore.RegisterPackage(prgrm)
	printer.PrinterOpcodesRegister(prgrm)
	printer.PrinterCompileErrorsRegister(prgrm)
	// http.RegisterPackage()
	regexp.RegisterPackage(prgrm)
	tcp.RegisterPackage(prgrm)
}

// MakeProgram returns a new empty program. Call RegisterPackages on it to add
// the native packages.
// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
// Now a constructor, site S1460.
func MakeProgram() *ast.CXProgram {
	minHeapSize := ast.MinHeapSize()
	state := &ast.CxIrState{}
	state.CxIrStateInit()
	// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
	// Now a heap literal, site S1461.
	newPrgrm := &ast.CXProgram{
		Packages: make(map[stringpool.StringGid]ast.CXPackageIndex, 0),
		// OBJMODEL: state.CXFunctionMng (manager storage) or []CXFunctionGid, phase 4 #351.
		// Now a slice of objects, site S1462.
		CXFunctions:    make([]ast.CXFunction, 0),
		CurrentPackage: -1,
		// OBJMODEL: state.CXCallMng (manager storage) or []CXCallGid, phase 4 #366.
		// Now a slice of objects, site S1463.
		CallStack: make([]ast.CXCall, constants.CALLSTACK_SIZE),
		Memory:    make([]byte, constants.STACK_SIZE+minHeapSize),
		Stack: ast.StackSegmentStruct{
			Size: constants.STACK_SIZE,
		},
		Data: ast.DataSegmentStruct{
			StartsAt: constants.STACK_SIZE,
		},
		Heap: ast.HeapSegmentStruct{
			Size:    minHeapSize,
			Pointer: constants.NULL_HEAP_ADDRESS_OFFSET, // We can start adding objects to the heap after the NULL (nil) bytes.
		},
		Version:   constants.LanguageVersionCurrent,
		CxIrState: state,
	}

	return newPrgrm
}
