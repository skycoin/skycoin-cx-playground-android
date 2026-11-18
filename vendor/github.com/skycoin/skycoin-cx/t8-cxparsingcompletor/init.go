package parsingcompletor

import (
	"github.com/skycoin/skycoin-cx/t4-actions"
	"github.com/skycoin/skycoin-cx/t4-opcodes"
	cxinit "github.com/skycoin/skycoin-cx/t7-init"
)

var Initialized bool

func init() {
	InitCXCore()
}

func InitCXCore() {
	if !Initialized {
		if actions.AST == nil {
			actions.AST = cxinit.MakeProgram()
		}
		opcodes.RegisterOpcodes(actions.AST)

		Initialized = true
	}
}
