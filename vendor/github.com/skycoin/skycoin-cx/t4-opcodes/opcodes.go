package opcodes

import (
	"fmt"

	coreassert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// RegisterPackage registers a package on the CX standard library. This does not create a `CXPackage` structure,
// it only tells the CX runtime that `pkgName` will exist by the time a CX program is run.
//func RegisterPackage(pkgName string) {
//	constants.CorePackages = append(constants.CorePackages, pkgName)
//}

// GetOpCodeCount returns an op code that is available for usage on the CX standard library.
/*
func GetOpCodeCount() int {
	return len(OpcodeHandlers)
}
*/

// RegisterOpCode registers handler on code, which must be the live row named
// name in constants.OpcodeLifecycleTable. Registering the same row again
// replaces it.
func RegisterOpCode(prgrm *ast.CXProgram, code int, name string, handler ast.OpcodeHandler, inputs []*ast.CXArgument, outputs []*ast.CXArgument) {
	row, found := constants.OpcodeLifecycleLiveByNumber(code)
	coreassert.AssertPreconditionTrue(found && row.Name == name, fmt.Sprintf("opcode %d %q is not a live row of constants.OpcodeLifecycleTable", code, name))
	if code >= len(ast.OpcodeHandlers) {
		ast.OpcodeHandlers = append(ast.OpcodeHandlers, make([]ast.OpcodeHandler, code+1)...)
	}
	ast.OpcodeHandlers[code] = handler

	ast.OpNames[code] = name
	ast.OpCodes[name] = code

	if inputs == nil {
		// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
		// Now a slice of objects, site S1344.
		inputs = []*ast.CXArgument{}
	}
	if outputs == nil {
		// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
		// Now a slice of objects, site S1345.
		outputs = []*ast.CXArgument{}
	}
	// OBJMODEL: state.CXNativeFunctionMng.CXNativeFunctionAlloc(), phase 4 #356.
	// Now a constructor call, site S1346.
	ast.Natives[code] = MakeNativeFunction(prgrm, code, inputs, outputs)
}

// OpcodeNumber returns the number of the live opcode name.
func OpcodeNumber(name string) int {
	row, found := constants.OpcodeLifecycleLiveByName(name)
	coreassert.AssertPreconditionTrue(found, fmt.Sprintf("opcode %q has no live row in constants.OpcodeLifecycleTable", name))
	return row.Number
}

// RegisterFunction registers the opcode name on its number in
// constants.OpcodeLifecycleTable.
func RegisterFunction(prgrm *ast.CXProgram, name string, handler ast.OpcodeHandler, inputs []*ast.CXArgument, outputs []*ast.CXArgument) {
	RegisterOpCode(prgrm, OpcodeNumber(name), name, handler, inputs, outputs)
}

// RegisterOperator registers the opcode name like RegisterFunction and makes
// it the operator for atomicType.
func RegisterOperator(prgrm *ast.CXProgram, name string, handler ast.OpcodeHandler, inputs []*ast.CXArgument, outputs []*ast.CXArgument, atomicType types.Code, operator int) {
	code := OpcodeNumber(name)
	RegisterOpCode(prgrm, code, name, handler, inputs, outputs)
	ast.Operators[ast.GetTypedOperatorOffset(atomicType, operator)] = ast.Natives[code]
}

// MakeNativeFunction ...
// OBJMODEL: state.CXNativeFunctionMng.CXNativeFunctionAlloc(), phase 4 #356.
// Now a constructor, site S1347.
func MakeNativeFunction(prgrm *ast.CXProgram, opCode int, inputs []*ast.CXArgument, outputs []*ast.CXArgument) *ast.CXNativeFunction {
	// OBJMODEL: state.CXNativeFunctionMng.CXNativeFunctionAlloc(), phase 4 #356.
	// Now a heap literal, site S1348.
	fn := &ast.CXNativeFunction{
		AtomicOPCode: opCode,
		// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
		// Now a slice of objects, site S1349.
		Inputs: []*ast.CXArgument{},
		// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
		// Now a slice of objects, site S1350.
		Outputs: []*ast.CXArgument{},
	}

	offset := types.Pointer(0)
	for _, inp := range inputs {
		inp.Offset = offset
		offset.Add(ast.GetNativeSize(prgrm, inp))
		fn.Inputs = append(fn.Inputs, inp)
	}
	for _, out := range outputs {
		out.Offset = offset
		offset.Add(ast.GetNativeSize(prgrm, out))
		fn.Outputs = append(fn.Outputs, out)
	}

	return fn
}
