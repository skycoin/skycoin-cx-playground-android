package ast

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// OpcodeHandler is the signature of an opcode or library function handler.
type OpcodeHandler func(prgrm *CXProgram, inputs []CXValue, outputs []CXValue)

// OpcodeDispatchFunction runs the opcode code on its operands and reports
// false for an opcode this binary does not link.
type OpcodeDispatchFunction func(prgrm *CXProgram, code int, inputs []CXValue, outputs []CXValue) bool

// OpcodeDispatch is the generated opcode switch the old loop calls. The
// registration of the binary sets it once, see CoreRegister and RegisterPackages.
// VIOLATION: state outside State. Owner: t3-ast; moves to CXNativeFunctionMng in phase 1.
var OpcodeDispatch OpcodeDispatchFunction = opcodeDispatchNone

func opcodeDispatchNone(prgrm *CXProgram, code int, inputs []CXValue, outputs []CXValue) bool {
	return false
}

// NativeFunctionHost returns the native of an opcode a program that embeds
// CX adds outside OpcodeSignatureTable, such as app.Rect of the playground,
// or nil. prgrm is nil when NativeLinked only asks whether code exists. Its
// OpcodeDispatch wrapper runs the opcode. Nil adds none.
// VIOLATION: state outside State. Owner: t3-ast; moves to CXNativeFunctionMng in phase 1.
var NativeFunctionHost func(prgrm *CXProgram, code int) *CXNativeFunction

// IsOperator reports whether opCode is an untyped operator.
func IsOperator(opCode int) bool {
	return opCode > constants.START_OF_OPERATORS && opCode < constants.END_OF_OPERATORS
}

func IsArithmeticOperator(opCode int) bool {
	return opCode > constants.START_OF_ARITHMETIC_OPERATORS && opCode < constants.END_OF_ARITHMETIC_OPERATORS
}

func IsComparisonOperator(opCode int) bool {
	return opCode > constants.START_OF_COMPARISON_OPERATORS && opCode < constants.END_OF_COMPARISON_OPERATORS
}

// NativeFunctionOf returns the built-in function code names, an opcode
// number or a library function key, nil when there is none or the binary
// does not link its library. An opcode's parameters come from
// constants.OpcodeSignatureTable and a library function's from
// constants.LibraryFunctionSignatureTable.
// OBJMODEL: state.CXNativeFunctionMng.CXNativeFunctionAlloc(), phase 4 #356.
// Now a constructor, site S1785.
func NativeFunctionOf(prgrm *CXProgram, code int) *CXNativeFunction {
	if code < 0 {
		library, function := constants.LibraryFunctionKeyParts(code)
		if !LibraryFunctionLinked(library, function) {
			return nil
		}
		librarySignature := constants.LibraryFunctionSignatureOf(library, function)
		// OBJMODEL: state.CXNativeFunctionMng.CXNativeFunctionAlloc(), phase 4 #356.
		// Now a heap literal, site S1789.
		return &CXNativeFunction{
			AtomicOPCode:          constants.OP_LIBRARY_CALL,
			LibraryNumber:         library,
			LibraryFunctionNumber: function,
			Inputs:                NativeParameters(prgrm, librarySignature.LibraryFunctionSignatureInputs()),
			Outputs:               NativeParameters(prgrm, librarySignature.LibraryFunctionSignatureOutputs()),
		}
	}
	signature := constants.OpcodeSignatureOf(code)
	if signature.OpcodeSignatureGroup == constants.OpcodeGroupNone {
		if NativeFunctionHost != nil {
			return NativeFunctionHost(prgrm, code)
		}
		return nil
	}
	// OBJMODEL: state.CXNativeFunctionMng.CXNativeFunctionAlloc(), phase 4 #356.
	// Now a heap literal, site S1786.
	return &CXNativeFunction{
		AtomicOPCode: code,
		Inputs:       NativeParameters(prgrm, signature.OpcodeSignatureInputs()),
		Outputs:      NativeParameters(prgrm, signature.OpcodeSignatureOutputs()),
	}
}

// NativeParameters makes the arguments of signature parameters. A native has
// no frame, so their Offset stays 0.
// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
// Now a constructor, site S1787.
func NativeParameters(prgrm *CXProgram, parameters []constants.OpcodeParameter) []*CXArgument {
	// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
	// Now a slice of objects, site S1788.
	args := make([]*CXArgument, 0, len(parameters))
	for _, parameter := range parameters {
		switch parameter.OpcodeParameterForm {
		case constants.OpcodeParameterFormSlice:
			args = append(args, Slice(parameter.OpcodeParameterType))
		case constants.OpcodeParameterFormStruct:
			structType := constants.OpcodeParameterStructTable[parameter.OpcodeParameterStruct]
			args = append(args, MakeStructParameter(prgrm, structType.OpcodeParameterStructPackage, structType.OpcodeParameterStructType, structType.OpcodeParameterStructName))
		default:
			args = append(args, Param(parameter.OpcodeParameterType))
		}
	}
	return args
}

// RuntimeOpcodeUnlinked is the panic value of an opcode the binary does not
// link. cx and a cx build core refuse such a program before it runs.
type RuntimeOpcodeUnlinked int
