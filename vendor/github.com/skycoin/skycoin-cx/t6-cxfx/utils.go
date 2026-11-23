//go:build cxfx

package cxfx

import (
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-execute" // thin: only Callback; no depth gain
)

type Func_i32_i32 func(a int32, b int32)

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CallbackMng in phase 1.
var Functions_i32_i32 []Func_i32_i32

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CStringMng in phase 1.
var freeFns map[string]*func() = make(map[string]*func(), 0)

// VIOLATION: state outside State. Owner: t6-cxfx; moves to CStringMng in phase 1.
var cSources map[string]**uint8 = make(map[string]**uint8, 0)

func opGlfwFuncI32I32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	packageNameGid, packageFound := prgrm.CXProgramNameFindBytes(ast.ProgramStringBytesRead(prgrm, &inputs[0]))
	functionIndex, functionFound := cxfxFunction(prgrm, packageNameGid, packageFound, ast.ProgramStringBytesRead(prgrm, &inputs[1]))
	callback := func(a int32, b int32) {
		var inps [][]byte = make([][]byte, 2)
		inps[0] = fromI32(a)
		inps[1] = fromI32(b)
		if functionFound {
			execute.Callback(prgrm, prgrm.GetFunctionFromArray(functionIndex), inps)
		}
	}

	Functions_i32_i32 = append(Functions_i32_i32, callback)
	outputs[0].Set_i32(prgrm, int32(len(Functions_i32_i32)-1))
}

func opGlfwCallI32I32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	index := inputs[0].Get_i32(prgrm)
	count := int32(len(Functions_i32_i32))
	if index >= 0 && index < count {
		Functions_i32_i32[index](inputs[1].Get_i32(prgrm), inputs[2].Get_i32(prgrm))
	}
}
