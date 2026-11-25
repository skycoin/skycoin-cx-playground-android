package constants

// LibraryFunctionSignature is the signature of one system library function.
// Its inputs and then its outputs are the rows of LibraryParameterTable from
// ParameterFirst on. Defined is false for a number without a definition row,
// and Vm is true for a function with a vm form.
type LibraryFunctionSignature struct {
	LibraryFunctionSignatureDefined        bool
	LibraryFunctionSignatureVm             bool
	LibraryFunctionSignatureParameterFirst uint16
	LibraryFunctionSignatureInputCount     uint8
	LibraryFunctionSignatureOutputCount    uint8
}

// LibraryFunctionSignatureOf returns the signature of function of library,
// with Defined false when it has no definition row.
func LibraryFunctionSignatureOf(library int, function int) LibraryFunctionSignature {
	if library <= 0 || library >= LibraryCount || function <= 0 || function >= LibraryFunctionTableSize {
		return LibraryFunctionSignature{}
	}
	return LibraryFunctionSignatureTable[library][function]
}

// LibraryFunctionSignatureInputs returns the input parameters of signature.
// The caller must not change them.
func (signature LibraryFunctionSignature) LibraryFunctionSignatureInputs() []OpcodeParameter {
	first := int(signature.LibraryFunctionSignatureParameterFirst)
	return LibraryParameterTable[first : first+int(signature.LibraryFunctionSignatureInputCount)]
}

// LibraryFunctionSignatureOutputs returns the output parameters of
// signature. The caller must not change them.
func (signature LibraryFunctionSignature) LibraryFunctionSignatureOutputs() []OpcodeParameter {
	first := int(signature.LibraryFunctionSignatureParameterFirst) + int(signature.LibraryFunctionSignatureInputCount)
	return LibraryParameterTable[first : first+int(signature.LibraryFunctionSignatureOutputCount)]
}
