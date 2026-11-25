package ast

// ProgramFormatBufferSize is the capacity ProgramFormatBuffer starts with. A
// longer print grows it once, and the program keeps the larger buffer.
const ProgramFormatBufferSize = 4096

// CXCallbackValues holds the operands of the native opcodes a nested loop of
// Callback runs. The outer loop is in the middle of an opcode with its own.
type CXCallbackValues struct {
	CallbackValuesInputs  []CXValue
	CallbackValuesOutputs []CXValue
}

// RunBuffersPrepare sizes the buffers the old loop reuses, before the run,
// so the run does not allocate them (#407). The operand buffers of the run
// and of a Callback loop take the largest arity of an expression, and the
// format buffer ProgramFormatBufferSize bytes.
func (cxprogram *CXProgram) RunBuffersPrepare() {
	inputCount, outputCount := cxprogram.runBuffersArity()
	if len(cxprogram.ProgramInputValues) < inputCount {
		cxprogram.ProgramInputValues = make([]CXValue, inputCount)
	}
	if len(cxprogram.ProgramOutputValues) < outputCount {
		cxprogram.ProgramOutputValues = make([]CXValue, outputCount)
	}
	if len(cxprogram.ProgramCallbackValues) == 0 {
		cxprogram.ProgramCallbackValues = make([]CXCallbackValues, 1)
	}
	cxprogram.ProgramCallbackValues[0].callbackValuesSize(inputCount, outputCount)
	if cap(cxprogram.ProgramFormatBuffer) < ProgramFormatBufferSize {
		cxprogram.ProgramFormatBuffer = make([]byte, 0, ProgramFormatBufferSize)
	}
}

// runBuffersArity returns the largest input and output counts of an
// expression.
func (cxprogram *CXProgram) runBuffersArity() (int, int) {
	inputCount, outputCount := 0, 0
	for i := range cxprogram.CXAtomicOps {
		inputCount = max(inputCount, len(cxprogram.CXAtomicOps[i].GetInputs(cxprogram)))
		outputCount = max(outputCount, len(cxprogram.CXAtomicOps[i].GetOutputs(cxprogram)))
	}
	return inputCount, outputCount
}

// callbackValuesSize makes the buffers hold inputCount and outputCount
// operands.
func (values *CXCallbackValues) callbackValuesSize(inputCount, outputCount int) {
	if len(values.CallbackValuesInputs) < inputCount {
		values.CallbackValuesInputs = make([]CXValue, inputCount)
	}
	if len(values.CallbackValuesOutputs) < outputCount {
		values.CallbackValuesOutputs = make([]CXValue, outputCount)
	}
}

// RunCallbackValues returns the operand buffers of the Callback loop at
// depth, 0 for a callback the run makes. A callback made inside a callback
// adds a set for its depth the first time, which the program keeps.
// VIOLATION VMALLOC-021: a callback inside a callback sizes the set of its depth once, #407.
func (cxprogram *CXProgram) RunCallbackValues(depth int) *CXCallbackValues {
	for len(cxprogram.ProgramCallbackValues) <= depth {
		inputCount, outputCount := cxprogram.runBuffersArity()
		var values CXCallbackValues
		values.callbackValuesSize(inputCount, outputCount)
		cxprogram.ProgramCallbackValues = append(cxprogram.ProgramCallbackValues, values)
	}
	return &cxprogram.ProgramCallbackValues[depth]
}
