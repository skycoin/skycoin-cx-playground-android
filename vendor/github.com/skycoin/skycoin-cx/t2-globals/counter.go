package globals

import (
	"github.com/skycoin/skycoin-cx/t1-constants" // thin: only END_OF_NAMED_OPCODES; core, keep
)

var (
	// Unused since opcode numbers are fixed in constants.OpcodeLifecycleTable.
	// Removing it drops t2-globals to tier 1, so it goes with that rename.
	OpCodeSystemCounter int = constants.END_OF_NAMED_OPCODES
)
