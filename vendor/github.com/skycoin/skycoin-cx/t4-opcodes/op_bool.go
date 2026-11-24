package opcodes

import (
	"strconv"

	// GCREACH: t3-ast owns the heap and the GC roots. This file reads and writes values
	// and allocates nothing, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

func opBoolPrint(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	printFormattedLine(prgrm, strconv.AppendBool(formatBuffer(prgrm), inputs[0].Get_bool(prgrm)))
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
