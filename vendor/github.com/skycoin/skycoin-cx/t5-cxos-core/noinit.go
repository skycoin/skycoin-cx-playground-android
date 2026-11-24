//go:build nocxos

package cxoscore

import (
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// RegisterPackage registers nothing in a cx built with nocxos.
func RegisterPackage(prgrm *ast.CXProgram) {}
