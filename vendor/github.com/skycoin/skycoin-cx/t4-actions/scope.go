package actions

import (
	"math"

	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// DefineNewScope marks the first and last non CX_LINE expressions of a block
// as the start and end of a scope. Nested blocks may share an expression.
func DefineNewScope(exprs []ast.CXExpression) {
	first, last := -1, -1
	for i := range exprs {
		if exprs[i].Type != ast.CX_LINE {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return
	}
	assert.AssertInvariantTrue(exprs[first].ExpressionScopeOpenCount < math.MaxUint8, "DefineNewScope: too many scopes open at one expression")
	assert.AssertInvariantTrue(exprs[last].ExpressionScopeCloseCount < math.MaxUint8, "DefineNewScope: too many scopes close at one expression")
	exprs[first].ExpressionScopeOpenCount++
	exprs[last].ExpressionScopeCloseCount++
}
