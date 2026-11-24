//go:build !nocxos

package cxoscore

import (
	"os"

	"github.com/skycoin/skycoin-cx/t3-ast"
)

func opOsExit(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	exitCode := inputs[0].Get_i32(prgrm)
	os.Exit(int(exitCode))
}
