package ast

// ProgramFormatBufferSize is the capacity ProgramFormatBuffer starts with. A
// longer print grows it once, and the program keeps the larger buffer.
const ProgramFormatBufferSize = 4096

// RunBuffersPrepare sizes the buffers the old loop reuses, before the run,
// so the run does not allocate them (#407). The operand buffers take the
// largest arity of an expression, and the format buffer
// ProgramFormatBufferSize bytes.
func (cxprogram *CXProgram) RunBuffersPrepare() {
	inputCount, outputCount := 0, 0
	for i := range cxprogram.CXAtomicOps {
		inputCount = max(inputCount, len(cxprogram.CXAtomicOps[i].GetInputs(cxprogram)))
		outputCount = max(outputCount, len(cxprogram.CXAtomicOps[i].GetOutputs(cxprogram)))
	}
	if len(cxprogram.ProgramInputValues) < inputCount {
		cxprogram.ProgramInputValues = make([]CXValue, inputCount)
	}
	if len(cxprogram.ProgramOutputValues) < outputCount {
		cxprogram.ProgramOutputValues = make([]CXValue, outputCount)
	}
	if cap(cxprogram.ProgramFormatBuffer) < ProgramFormatBufferSize {
		cxprogram.ProgramFormatBuffer = make([]byte, 0, ProgramFormatBufferSize)
	}
}
