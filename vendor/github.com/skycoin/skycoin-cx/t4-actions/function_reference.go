package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// functionReferenceInputsResolve replaces each argument passed where a native
// takes a function reference (ast.ConstCxArg_FUNC) with an i32 literal holding
// the CXFunctionIndex of the function it names, so the opcode reads the
// function without a name lookup at run time (#432). An argument that is not
// a function name is a compile error.
func functionReferenceInputsResolve(prgrm *ast.CXProgram, expression *ast.CXAtomicOperator) {
	operator := prgrm.GetFunctionFromArray(expression.Operator)
	if operator == nil || !operator.IsBuiltIn() || expression.Inputs == nil {
		return
	}
	expected := operator.GetInputs(prgrm)
	received := expression.Inputs.Fields
	for i := 0; i < len(expected) && i < len(received); i++ {
		if !functionReferenceParameter(prgrm, expected[i]) {
			continue
		}
		receivedTypeSig := prgrm.GetCXTypeSignatureFromArray(received[i])
		functionIndex, found := functionReferenceTarget(prgrm, receivedTypeSig)
		if !found && !functionReferenceUnresolved(prgrm, receivedTypeSig) {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeFunctionReferenceInput, receivedTypeSig.ArgDetails, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(expressionOperatorName(prgrm, operator)), int64(i + 1), prgrm.CXProgramCompileErrorText(valueFormattedType(prgrm, receivedTypeSig))})
			continue
		}
		if !found {
			continue
		}
		var literal [4]byte
		types.Write_i32(literal[:], 0, int32(functionIndex))
		received[i] = ast.AllocCXTypeSignatureOfCXArgBuilder(prgrm, WritePrimary(prgrm, types.I32, literal[:], false))
	}
}

// functionReferenceParameter reports a native input declared as a function
// reference.
func functionReferenceParameter(prgrm *ast.CXProgram, typeSigIdx ast.CXTypeSignatureIndex) bool {
	typeSig := prgrm.GetCXTypeSignatureFromArray(typeSigIdx)
	if typeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return false
	}
	return prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta)).Type == types.FUNC
}

// functionReferenceUnresolved reports a name that is neither a variable nor a
// function, which lookupSymbol has reported already.
func functionReferenceUnresolved(prgrm *ast.CXProgram, typeSig *ast.CXTypeSignature) bool {
	if typeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return false
	}
	return prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta)).Type == types.IDENTIFIER
}

// functionReferenceTarget returns the function an argument names, the
// argument lookupSymbol typed as a function when it found no variable.
func functionReferenceTarget(prgrm *ast.CXProgram, typeSig *ast.CXTypeSignature) (ast.CXFunctionIndex, bool) {
	if typeSig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return -1, false
	}
	arg := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(typeSig.Meta))
	if arg.Type != types.FUNC || len(arg.Fields) != 0 || len(arg.Indexes) != 0 {
		return -1, false
	}
	pkg, err := prgrm.GetPackageFromArray(arg.Package)
	if err != nil {
		return -1, false
	}
	fn, err := pkg.GetFunction(prgrm, StripNameNumber(prgrm, arg.Name))
	if err != nil {
		return -1, false
	}
	return ast.CXFunctionIndex(fn.Index), true
}
