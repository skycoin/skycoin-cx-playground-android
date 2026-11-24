package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
)

var (
	// VIOLATION: state outside State. Owner: t3-ast; moves to CXNativeFunctionMng in phase 1.
	OpcodeHandlers []OpcodeHandler
)

// OpcodeHandler ...
// TODO: make special op-code handler for 2 input, 1 output atomics
type OpcodeHandler func(prgrm *CXProgram, inputs []CXValue, outputs []CXValue)

//TODO: Do atomic opcode handlers (not slices, not arrays)
//type AtomicOpcodeHandler func(input1 CXValue, input2 CXValue, output CXValue)

// TODO: RENAME THIS. The nameing is very bad
const (
	OPERATOR_COUNT         = constants.END_OF_OPERATORS - constants.START_OF_OPERATORS + 1
	OPERATOR_HANDLER_COUNT = types.COUNT * OPERATOR_COUNT
)

// Todo: Rename Natives
// Todo: What is an operator?
var (
	// Natives ...
	// OBJMODEL: state.CXNativeFunctionMng, keyed by opcode, phase 4 #356.
	// Now a pointer at rest, site S0387.
	Natives = map[int]*CXNativeFunction{}
	// OBJMODEL: state.CXNativeFunctionMng, keyed by opcode, phase 4 #356.
	// Now a pointer at rest, site S0388.
	Operators []*CXNativeFunction
)

// GetOpCodeCount returns an op code that is available for usage on the CX standard library.
/*
func GetOpCodeCount() int {
	return len(OpcodeHandlers)
}
*/

// TODO: WHAT IS AN "OPERATOR"
func IsOperator(opCode int) bool {
	return opCode > constants.START_OF_OPERATORS && opCode < constants.END_OF_OPERATORS
}

func IsArithmeticOperator(opCode int) bool {
	return opCode > constants.START_OF_ARITHMETIC_OPERATORS && opCode < constants.END_OF_ARITHMETIC_OPERATORS
}

func IsComparisonOperator(opCode int) bool {
	return opCode > constants.START_OF_COMPARISON_OPERATORS && opCode < constants.END_OF_COMPARISON_OPERATORS
}

func GetTypedOperatorOffset(typeCode types.Code, opCode int) int {
	return int(typeCode)*OPERATOR_COUNT + opCode - constants.START_OF_OPERATORS - 1
}

func GetTypedOperator(typeCode types.Code, opCode int) *CXNativeFunction {
	return Operators[GetTypedOperatorOffset(typeCode, opCode)]
}
