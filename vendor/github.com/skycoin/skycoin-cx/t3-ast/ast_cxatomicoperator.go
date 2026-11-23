package ast

import stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"

// ----------------------------------------------------------------
//                             `CXAtomicOperator` Getters

func (op *CXAtomicOperator) GetOperatorName(prgrm *CXProgram) string {
	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0089.
	opOperator := prgrm.GetFunctionFromArray(op.Operator)
	if opOperator.IsBuiltIn() {
		return OpNames[opOperator.AtomicOPCode]
	}
	return prgrm.CXProgramNameText(opOperator.Name)

}

// ----------------------------------------------------------------
//                     `CXAtomicOperator` Member handling

// AddInput ...
func (op *CXAtomicOperator) AddInput(prgrm *CXProgram, typeSignatureIdx CXTypeSignatureIndex) *CXAtomicOperator {
	if op.Inputs == nil {
		// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
		// Now a heap literal, site S0090.
		op.Inputs = &CXStruct{Package: op.Package}
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0091.
	typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
	if typeSignature.Package == -1 {
		typeSignature.Package = op.Package
	}

	// Add Package if arg has no package
	if typeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0092.
		arg := prgrm.GetCXArgFromArray(CXArgumentIndex(typeSignature.Meta))
		if arg.Package == -1 {
			arg.Package = op.Package
		}
	}

	op.Inputs.AddField_CXAtomicOps(prgrm, typeSignatureIdx)

	return op
}

func (op *CXAtomicOperator) GetInputs(prgrm *CXProgram) []CXTypeSignatureIndex {
	if op == nil || op.Inputs == nil {
		return []CXTypeSignatureIndex{}
	}

	return op.Inputs.Fields
}

// RemoveInput ...
// func (op *CXAtomicOperator) RemoveInput() {
// 	if len(op.Inputs) > 0 {
// 		op.Inputs = op.Inputs[:len(op.Inputs)-1]
// 	}
// }

// AddOutput ...
func (op *CXAtomicOperator) AddOutput(prgrm *CXProgram, typeSignatureIdx CXTypeSignatureIndex) *CXAtomicOperator {
	if op.Outputs == nil {
		// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
		// Now a heap literal, site S0093.
		op.Outputs = &CXStruct{Package: op.Package}
	}

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0094.
	typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
	if typeSignature.Package == -1 {
		typeSignature.Package = op.Package
	}

	// Add Package if arg has no package
	if typeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0095.
		arg := prgrm.GetCXArgFromArray(CXArgumentIndex(typeSignature.Meta))
		if arg.Package == -1 {
			arg.Package = op.Package
		}
	}

	op.Outputs.AddField_CXAtomicOps(prgrm, typeSignatureIdx)
	return op
}

func (op *CXAtomicOperator) GetOutputs(prgrm *CXProgram) []CXTypeSignatureIndex {
	if op == nil || op.Outputs == nil {
		return []CXTypeSignatureIndex{}
	}

	return op.Outputs.Fields
}

// RemoveOutput ...
// func (op *CXAtomicOperator) RemoveOutput() {
// 	if len(op.Outputs) > 0 {
// 		op.Outputs = op.Outputs[:len(op.Outputs)-1]
// 	}
// }

// AddLabel ...
func (op *CXAtomicOperator) AddLabel(lbl stringpool.StringGid) *CXAtomicOperator {
	op.Label = lbl
	return op
}
