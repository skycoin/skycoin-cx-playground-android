package execute

import (
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	// GCREACH: t3-ast owns the heap and the GC roots. Callback runs a nested old loop
	// inside a native opcode, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

//TODO: Define Function Pointers and deprecate Callback

// What calls Callback?ex
// Callback is only called from opHttpHandle, can probably be removed
// TODO: Delete and delete call from opHTTPHandle
// TODO: We probably dont need this? HTTPHandle can work in another way
// TODO: Is Callback actually "CallFunction" ?
func Callback(cxprogram *ast.CXProgram, fn *ast.CXFunction, inputs [][]byte) (outputs [][]byte) {
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

	// The nested loop has its own operand buffers, as the outer loop is in the
	// middle of an opcode with its own.
	// VIOLATION VMALLOC-021: the nested buffers grow on the first opcodes, #407.
	var nestedInputs, nestedOutputs []ast.CXValue
	err := runLoop(cxprogram, true, maxOps, previousCall, &nestedInputs, &nestedOutputs)
	if err != nil {
		os.Exit(types.CX_INTERNAL_ERROR)
	}

	cxprogram.CallCounter = previousCall
	cxprogram.CallStack[cxprogram.CallCounter].Line = line

	fnOutputs := fn.GetOutputs(cxprogram)
	for _, outputIdx := range fnOutputs {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S1248.
		output := cxprogram.GetCXTypeSignatureFromArray(outputIdx)

		// Making a copy of the bytes, so if we modify the bytes being held by `outputs`
		// we don't modify the program memory.
		mem := types.GetSlice_byte(cxprogram.Memory, ast.GetFinalOffset(cxprogram, newFP, nil, output), output.GetSize(cxprogram, false))
		cop := make([]byte, len(mem))
		copy(cop, mem)
		outputs = append(outputs, cop)
	}
	return outputs
}
