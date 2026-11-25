package init

import (
	"github.com/skycoin/skycoin-cx/t1-constants" // thin: only CALLSTACK_SIZE, NULL_HEAP_ADDRESS_OFFSET, OpcodeGroupLinked, STACK_SIZE; no depth gain
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-cx-runtime-printer"   // thin: only PrinterCompileErrorsRegister; registration list, keep
	"github.com/skycoin/skycoin-cx/t5-cipher"               // thin: only RegisterPackage; registration list, keep
	core "github.com/skycoin/skycoin-cx/t5-cx-runtime-core" // thin: only CoreOpcodeDispatch; the opcode switch, keep
	"github.com/skycoin/skycoin-cx/t5-cxnet"                // thin: only RegisterPackage; registration list, keep
	"github.com/skycoin/skycoin-cx/t5-cxos"                 // thin: only RegisterPackage; registration list, keep
	"github.com/skycoin/skycoin-cx/t6-cxfx"                 // thin: only CxfxOpcodeDispatch; the opcode switch, keep

	// "github.com/skycoin/cx/cx/packages/http"

	"github.com/skycoin/skycoin-cx/t4-regexp" // thin: only RegisterPackage; registration list, keep
)

// RegisterPackages adds the native packages to prgrm, registers the system
// libraries and sets the opcode switch of cx.
func RegisterPackages(prgrm *ast.CXProgram) {
	ast.OpcodeDispatch = core.CoreOpcodeDispatch
	if constants.OpcodeGroupLinked(constants.OpcodeGroupCxfx) {
		ast.OpcodeDispatch = OpcodeDispatch
	}
	cipher.RegisterPackage(prgrm)
	cxos.RegisterPackage(prgrm)
	printer.PrinterCompileErrorsRegister(prgrm)
	// http.RegisterPackage()
	regexp.RegisterPackage(prgrm)
	cxnet.RegisterPackage(prgrm)
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
			Max:     constants.MAX_HEAP_SIZE,
			Pointer: constants.NULL_HEAP_ADDRESS_OFFSET, // We can start adding objects to the heap after the NULL (nil) bytes.
		},
		Version:   constants.LanguageVersionCurrent,
		CxIrState: state,
	}

	return newPrgrm
}

// OpcodeDispatch is the opcode switch of cx, the core switch and then the
// cxfx switch, which reports false in a build without the cxfx tag.
func OpcodeDispatch(prgrm *ast.CXProgram, code int, inputs []ast.CXValue, outputs []ast.CXValue) bool {
	return core.CoreOpcodeDispatch(prgrm, code, inputs, outputs) || cxfx.CxfxOpcodeDispatch(prgrm, code, inputs, outputs)
}
