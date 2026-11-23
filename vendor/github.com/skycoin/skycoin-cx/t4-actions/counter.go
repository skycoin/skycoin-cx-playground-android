package actions

import (
	"strconv"

	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

var (
	// VIOLATION: state outside State. Owner: t4-actions; moves to ParseMng in phase 2.
	TemporaryVariableCounter int
)

// generateTempVarName interns the name prefix_N of the next temporary
// variable in the string pool of prgrm.
func generateTempVarName(prgrm *ast.CXProgram, prefix string) stringpool.StringGid {
	var buf [64]byte
	name := append(buf[:0], prefix...)
	name = append(name, '_')
	name = strconv.AppendInt(name, int64(TemporaryVariableCounter), 10)
	TemporaryVariableCounter++

	return prgrm.CxIrState.StringPool.StringPoolIntern(name)
}
