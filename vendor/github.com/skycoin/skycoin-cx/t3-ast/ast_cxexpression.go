package ast

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

/*
 * CXEXPR_TYPE enum contains CX expressions types for CXExpression struct
 */
type CXEXPR_TYPE int

const (
	CXEXPR_UNUSED CXEXPR_TYPE = iota
	CXEXPR_METHOD_CALL
	CXEXPR_STRUCT_LITERAL
	CXEXPR_ARRAY_LITERAL
	// CXEXPR_FUNCTION_CALL is a resolved call, which cannot be called again.
	CXEXPR_FUNCTION_CALL
	// CXEXPR_STRUCT_LITERAL_FIELD writes one field of a struct literal that is not
	// yet given its variable.
	CXEXPR_STRUCT_LITERAL_FIELD
)

// String returns alias for constants defined for cx edpression type
func (cxet CXEXPR_TYPE) String() string {
	return [...]string{"Unused", "MethodCall", "StructLiteral", "ArrayLiteral", "FunctionCall", "StructLiteralField"}[int(cxet)]
}

// IsMethodCall checks if expression type is method call
func (cxe CXExpression) IsMethodCall() bool {
	return cxe.ExpressionType == CXEXPR_METHOD_CALL
}

// IsStructLiteral checks if expression type is struct literal
func (cxe CXExpression) IsStructLiteral() bool {
	return cxe.ExpressionType == CXEXPR_STRUCT_LITERAL
}

// IsArrayLiteral checks if expression type is array literal
func (cxe CXExpression) IsArrayLiteral() bool {
	return cxe.ExpressionType == CXEXPR_ARRAY_LITERAL
}

// IsBreak checks if expression type is break
func (cxe CXExpression) IsBreak(prgrm *CXProgram) bool {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0120.
	expression, err := prgrm.GetCXAtomicOp(cxe.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0121.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	return expressionOperator != nil && expressionOperator.AtomicOPCode == constants.OP_BREAK
}

// IsContinue checks if expression type is continue
func (cxe CXExpression) IsContinue(prgrm *CXProgram) bool {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0122.
	expression, err := prgrm.GetCXAtomicOp(cxe.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0123.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	return expressionOperator != nil && expressionOperator.AtomicOPCode == constants.OP_CONTINUE
}

// IsUndType checks if expression type is und type
func (cxe CXExpression) IsUndType(prgrm *CXProgram) bool {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0124.
	expression, err := prgrm.GetCXAtomicOp(cxe.Index)
	if err != nil {
		panic(err)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0125.
	expressionOperator := prgrm.GetFunctionFromArray(expression.Operator)
	return expressionOperator != nil && IsOperator(expressionOperator.AtomicOPCode)

}

// ----------------------------------------------------------------
//                             `CXExpression` Getters

func (cxe CXExpression) GetLabel(prgrm *CXProgram) stringpool.StringGid {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S0126.
	expression, err := prgrm.GetCXAtomicOp(cxe.Index)
	if err != nil {
		panic(err)
	}

	return expression.Label
}
