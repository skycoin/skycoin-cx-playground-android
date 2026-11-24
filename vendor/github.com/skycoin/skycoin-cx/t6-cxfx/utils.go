//go:build cxfx

package cxfx

import (
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-execute" // thin: only Callback; no depth gain
)

// opGlfwFuncI32I32 keeps the function the compiler resolved in the callback
// table and returns its handle for glfw.InvokeCallbackI32I32.
func opGlfwFuncI32I32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, cxfxCallbackI32I32Add(ast.CXFunctionIndex(inputs[0].Get_i32(prgrm))))
}

func opGlfwCallI32I32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	function := cxfxCallbackI32I32Get(inputs[0].Get_i32(prgrm))
	cxfxCallbackInputsI32(0, inputs[1].Get_i32(prgrm))
	cxfxCallbackInputsI32(1, inputs[2].Get_i32(prgrm))
	execute.Callback(prgrm, prgrm.GetFunctionFromArray(function), cxfxState.CxfxCallbackInputs[:2])
}
