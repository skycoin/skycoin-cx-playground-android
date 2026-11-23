package ast

// CXExpression is used represent a CX expression.
//
// All statements in CX are expressions, including for loops and other control
// flow.
type CXExpression struct {
	ExpressionType CXEXPR_TYPE

	// ExpressionScopeOpenCount and ExpressionScopeCloseCount are the number
	// of block scopes that start and end at this expression.
	ExpressionScopeOpenCount  uint8
	ExpressionScopeCloseCount uint8

	// For new CX AST Format
	Index int
	Type  CXOPERATION_TYPE
}
