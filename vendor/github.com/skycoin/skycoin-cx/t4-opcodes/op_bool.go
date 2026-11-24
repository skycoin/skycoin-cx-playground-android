package opcodes

import (
	"fmt"

	// GCREACH: t3-ast owns the heap and the GC roots. This file reads and writes values
	// and allocates nothing, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// VIOLATION STRHOT-081: O, the opcode works on Go string copies of CX strs, #412.
func opBoolPrint(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	fmt.Println(inputs[0].Get_bool(prgrm))
}

func opBoolEqual(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_bool(prgrm) == inputs[1].Get_bool(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

func opBoolUnequal(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_bool(prgrm) != inputs[1].Get_bool(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

func opBoolNot(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := !inputs[0].Get_bool(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

func opBoolAnd(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_bool(prgrm)
	inpV1 := inputs[1].Get_bool(prgrm)
	outputs[0].Set_bool(prgrm, inpV0 && inpV1)
}

func opBoolOr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_bool(prgrm)
	inpV1 := inputs[1].Get_bool(prgrm)
	outputs[0].Set_bool(prgrm, inpV0 || inpV1)
}
