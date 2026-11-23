package opcodes

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// opSerialize returns the whole running program, memory included, as a
// []ui8. The affordance input does not narrow what is serialized yet.
func opSerialize(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	byts, err := ast.SerializeCXProgram(prgrm, true, nil)
	if err != nil {
		panic(types.CX_RUNTIME_NOT_IMPLEMENTED)
	}
	count := types.Cast_int_to_ptr(len(byts))
	if count > ast.SliceElementLimit(1) {
		panic(constants.HEAP_EXHAUSTED_ERROR)
	}
	slice := ast.SliceResizeEx(prgrm, 0, count, 1)
	copy(ast.GetSliceData(prgrm, slice, 1), byts)
	outputs[0].Set_ptr(prgrm, slice)
}

// opDeserialize checks that a []ui8 holds a program this cx can load. Bytes
// that do not are a runtime error. The program itself is not kept.
func opDeserialize(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	byts := inputs[0].GetSlice_ui8(prgrm)
	// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
	// Now a constructor call, site S1309.
	if _, err := ast.Deserialize(byts, nil); err != nil {
		panic(types.CX_RUNTIME_INVALID_ARGUMENT)
	}
}
