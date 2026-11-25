package opcodes

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
	// GCREACH: t3-ast owns the heap and the GC roots. This file reads and writes values
	// and allocates nothing, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

func OpIdentity(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var out1 *ast.CXArgument
	if outputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1302.
		out1 = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outputs[0].TypeSignature.Meta))

		var elt *ast.CXArgument
		if len(out1.Fields) > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1303.
			elt = prgrm.GetCXArgFromArray(out1.Fields[len(out1.Fields)-1])
		} else {
			elt = out1
		}

		if ast.CXArgumentStringSlicePointer(elt) {
			if ast.CXTypeSignaturePassBy(prgrm, inputs[0].TypeSignature) == constants.PASSBY_REFERENCE {
				outputs[0].Set_ptr(prgrm, ast.PointerOfAddress(prgrm, inputs[0].Offset))
			} else {
				outputs[0].Set_bytes(prgrm, inputs[0].Get_bytes(prgrm))
			}
		} else if ast.CXArgumentTakesAddress(prgrm, elt, inputs[0].TypeSignature) {
			outputs[0].Set_ptr(prgrm, ast.PointerOfAddress(prgrm, inputs[0].Offset))
		} else {
			// Pass by value
			outputs[0].Set_bytes(prgrm, inputs[0].Get_bytes(prgrm))
		}
	} else if outputs[0].TypeSignature.Type == ast.TYPE_ATOMIC {
		if outputs[0].TypeSignature.PassBy == constants.PASSBY_REFERENCE {
			outputs[0].Set_ptr(prgrm, ast.PointerOfAddress(prgrm, inputs[0].Offset))
		} else {
			outputs[0].Set_bytes(prgrm, inputs[0].Get_bytes(prgrm))
		}
	} else if outputs[0].TypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
		if ast.CXTypeSignaturePointerValue(prgrm, inputs[0].TypeSignature) {
			outputs[0].Set_bytes(prgrm, inputs[0].Get_bytes(prgrm))
		} else {
			outputs[0].Set_ptr(prgrm, ast.PointerOfAddress(prgrm, inputs[0].Offset))
		}
	} else if outputs[0].TypeSignature.Type == ast.TYPE_ARRAY_ATOMIC {
		outputs[0].Set_bytes(prgrm, inputs[0].Get_bytes(prgrm))
	} else if outputs[0].TypeSignature.Type == ast.TYPE_POINTER_ARRAY_ATOMIC || outputs[0].TypeSignature.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
		if inputs[0].TypeSignature.PassBy == constants.PASSBY_REFERENCE {
			outputs[0].Set_ptr(prgrm, ast.PointerOfAddress(prgrm, inputs[0].Offset))
		} else {
			outputs[0].Set_bytes(prgrm, inputs[0].Get_bytes(prgrm))
		}
	} else if outputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		if outputs[0].TypeSignature.PassBy == constants.PASSBY_REFERENCE {
			outputs[0].Set_ptr(prgrm, ast.PointerOfAddress(prgrm, inputs[0].Offset))
		} else {
			outputs[0].Set_bytes(prgrm, inputs[0].Get_bytes(prgrm))
		}
	} else if outputs[0].TypeSignature.Type == ast.TYPE_STRUCT || outputs[0].TypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		if ast.CXTypeSignatureStructTakesAddress(prgrm, outputs[0].TypeSignature, inputs[0].TypeSignature) {
			outputs[0].Set_ptr(prgrm, ast.PointerOfAddress(prgrm, inputs[0].Offset))
		} else {
			outputs[0].Set_bytes(prgrm, inputs[0].Get_bytes(prgrm))
		}
	} else {
		panic("type is not known")
	}
}

func OpGoto(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := call.Operator.Expressions[call.Line]

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1304.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	call.Line = call.Line + cxAtomicOp.ThenLines
}

func OpJmp(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1305.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	if inputs[0].Get_bool(prgrm) {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}

func OpAbsJmp(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1306.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	if inputs[0].Get_bool(prgrm) {
		call.Line = cxAtomicOp.ThenLines
	} else {
		call.Line = cxAtomicOp.ElseLines
	}
}

func OpBreak(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := call.Operator.Expressions[call.Line]

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1307.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	call.Line = call.Line + cxAtomicOp.ThenLines
}

func OpContinue(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := call.Operator.Expressions[call.Line]

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1308.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	call.Line = call.Line + cxAtomicOp.ThenLines
}

func OpNop(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	// No Operation
	// Do Nothing
}

func OpPtrEq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, inputs[0].Get_ptr(prgrm) == inputs[1].Get_ptr(prgrm))
}

func OpPtrUneq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, inputs[0].Get_ptr(prgrm) != inputs[1].Get_ptr(prgrm))
}
