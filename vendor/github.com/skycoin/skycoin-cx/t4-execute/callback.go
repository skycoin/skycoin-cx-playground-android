package execute

import (
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	// GCREACH: t3-ast owns the heap and the GC roots. Callback runs a nested old loop
	// inside a native opcode, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// Callback runs fn with inputs written to its frame in a nested loop, for
// the callbacks of t6-cxfx. Its results are left in the frame.
func Callback(cxprogram *ast.CXProgram, fn *ast.CXFunction, inputs [][]byte) {
	line := cxprogram.CallStack[cxprogram.CallCounter].Line
	previousCall := cxprogram.CallCounter
	cxprogram.CallCounter++
	newCall := &cxprogram.CallStack[cxprogram.CallCounter]
	newCall.Operator = fn
	newCall.Line = 0
	newCall.FramePointer = cxprogram.Stack.Pointer
	cxprogram.Stack.Pointer += fn.Size
	newFP := newCall.FramePointer

	// wiping next mem frame (removing garbage)
	for c := types.Pointer(0); c < fn.Size; c++ {
		cxprogram.Memory[newFP+c] = 0
	}

	newCallOperatorInputs := newCall.Operator.GetInputs(cxprogram)
	for i, inp := range inputs {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1247.
		newCallOperatorInputTypeSig := cxprogram.GetCXTypeSignatureFromArray(newCallOperatorInputs[i])
		types.WriteSlice_byte(cxprogram.Memory, ast.GetFinalOffset(cxprogram, newFP, nil, newCallOperatorInputTypeSig), inp)
	}

	var maxOps = 0

	// The nested loop has its own operand buffers, sized before the run, as
	// the outer loop is in the middle of an opcode with its own (#407).
	values := cxprogram.RunCallbackValues(cxprogram.ProgramCallbackDepth)
	cxprogram.ProgramCallbackDepth++
	err := runLoop(cxprogram, true, maxOps, previousCall, &values.CallbackValuesInputs, &values.CallbackValuesOutputs)
	cxprogram.ProgramCallbackDepth--
	if err != nil {
		os.Exit(types.CX_INTERNAL_ERROR)
	}

	cxprogram.CallCounter = previousCall
	cxprogram.CallStack[cxprogram.CallCounter].Line = line
}
