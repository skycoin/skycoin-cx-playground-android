package opcodes

import (
	"bytes"
	"fmt"

	"github.com/skycoin/skycoin-cx/t0-types"
	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates strs, which
	// adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// VIOLATION: state outside State. Owner: t4-opcodes; moves to TestMng in phase 2.
var assertSuccess = true

// AssertFailed ...
func AssertFailed() bool {
	return !assertSuccess
}

// TODO: Rework
// VIOLATION STRHOT-118: E, the text of a failed assert, built only when it fails, #412.
func assert(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) (same bool) {
	var byts1, byts2 []byte
	var inp0 *ast.CXArgument
	if inputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1341.
		inp0 = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[0].TypeSignature.Meta))

		if inp0.Type == types.STR || inp0.PointerTargetType == types.STR {
			byts1 = inputs[0].Get_str_bytes(prgrm)
			byts2 = inputs[1].Get_str_bytes(prgrm)
		} else {
			byts1 = inputs[0].Get_bytes(prgrm)
			byts2 = inputs[1].Get_bytes(prgrm)
		}

	} else if inputs[0].TypeSignature.Type == ast.TYPE_ATOMIC || inputs[0].TypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
		byts1 = inputs[0].Get_bytes(prgrm)
		byts2 = inputs[1].Get_bytes(prgrm)
	} else if inputs[0].TypeSignature.Type == ast.TYPE_ARRAY_ATOMIC {
		byts1 = inputs[0].Get_bytes(prgrm)
		byts2 = inputs[1].Get_bytes(prgrm)
	} else if inputs[0].TypeSignature.Type == ast.TYPE_POINTER_ARRAY_ATOMIC {
		byts1 = inputs[0].Get_bytes(prgrm)
		byts2 = inputs[1].Get_bytes(prgrm)
	} else if inputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		byts1 = inputs[0].Get_bytes(prgrm)
		byts2 = inputs[1].Get_bytes(prgrm)
	} else if inputs[0].TypeSignature.Type == ast.TYPE_STRUCT || inputs[0].TypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		var isStr bool = false
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S1342.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(inputs[0].TypeSignature.Meta)

		lenFlds := len(structDetails.Fields)
		if lenFlds > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1343.
			fld := prgrm.GetCXArgFromArray(structDetails.Fields[lenFlds-1])
			if fld.Type == types.STR || fld.PointerTargetType == types.STR {
				isStr = true
			}
		}

		if isStr {
			byts1 = inputs[0].Get_str_bytes(prgrm)
			byts2 = inputs[1].Get_str_bytes(prgrm)
		} else {
			byts1 = inputs[0].Get_bytes(prgrm)
			byts2 = inputs[1].Get_bytes(prgrm)
		}
	} else {
		panic("type is not known")
	}

	same = bytes.Equal(byts1, byts2)
	if !same {
		message := inputs[2].Get_str_bytes(prgrm)
		call := prgrm.GetCurrentCall()
		// expr := call.Operator.Expressions[call.Line]
		cxLine, _ := prgrm.GetPreviousCXLine(call.Operator.Expressions, call.Line)

		if len(message) != 0 {
			fmt.Printf("%s: %d: result was not equal to the expected value; %s\n", prgrm.CXProgramSourceFileText(cxLine.FileName), cxLine.LineNumber, message)
		} else {
			fmt.Printf("%s: %d: result was not equal to the expected value\n", prgrm.CXProgramSourceFileText(cxLine.FileName), cxLine.LineNumber)
		}
	}

	assertSuccess = assertSuccess && same
	return same
}

func opAssertValue(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	same := assert(prgrm, inputs, outputs)
	outputs[0].Set_bool(prgrm, same)
}

func opTest(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	assert(prgrm, inputs, outputs)
}

func opPanic(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	if !assert(prgrm, inputs, outputs) {
		panic(types.CX_ASSERT)
	}
}

// panicIf/panicIfNot implementation
func panicIf(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue, condition bool) {
	if inputs[0].Get_bool(prgrm) == condition {
		// The printer prints the line and the message after the loop.
		prgrm.RuntimeError.RuntimeErrorRecordAssertLine = true
		prgrm.RuntimeError.RuntimeErrorRecordAssertMessage = inputs[1].Get_ptr(prgrm)
		panic(types.CX_ASSERT)
	}
}

// panic with CX_ASSERT exit code if condition is true
func opPanicIf(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	panicIf(prgrm, inputs, outputs, true)
}

// panic with CX_ASSERT exit code if condition is false
func opPanicIfNot(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	panicIf(prgrm, inputs, outputs, false)
}

// VIOLATION STRHOT-120: B, the text of an error code, #412.
func opStrError(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str(prgrm, ast.ErrorString(int(inputs[0].Get_i32(prgrm))))
}
