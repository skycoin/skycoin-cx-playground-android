package actions

import (
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// OBJMODEL: *CxIrState passed as a parameter, owned by main, phase 5 #372.
// Now a pointer at rest, site S0604.
var AST *ast.CXProgram

// VIOLATION: state outside State. Owner: t4-actions; moves to ParseMng in phase 2.
var CurrentFile ast.SourceFileGid

// VIOLATION: state outside State. Owner: t4-actions; moves to ParseMng in phase 2.
var LineNo int

// VIOLATION: state outside State. Owner: t4-actions; moves to ParseMng in phase 2.
var LineStr string

// MakeCXLineExpressionCurrent makes the CX_LINE marker of the line the parser
// is at, interning its text.
func MakeCXLineExpressionCurrent(prgrm *ast.CXProgram) *ast.CXExpression {
	return ast.MakeCXLineExpression(prgrm, CurrentFile, LineNo, prgrm.CXProgramNameIntern(LineStr))
}
