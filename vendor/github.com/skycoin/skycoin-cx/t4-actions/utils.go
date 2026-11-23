package actions

import (
	"bytes"

	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// StripNameNumber returns name without the n: prefix UpdateSymbolsTable gives
// a local variable.
func StripNameNumber(prgrm *ast.CXProgram, name stringpool.StringGid) stringpool.StringGid {
	text := prgrm.CXProgramNameBytes(name)
	index := bytes.IndexByte(text, ':')
	if index == -1 {
		return name
	}

	return prgrm.CxIrState.StringPool.StringPoolIntern(text[index+1:])
}
