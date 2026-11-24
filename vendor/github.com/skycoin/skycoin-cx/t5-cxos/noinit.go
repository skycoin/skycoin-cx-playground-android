//go:build nocxos

package cxos

import (
	"github.com/skycoin/skycoin-cx/t3-ast"
)

func RegisterPackage(prgrm *ast.CXProgram) {

}

func InitConstants(constantAdd func(code int, name string, value int32)) {}

// LibraryFileRegister registers nothing in a cx built with nocxos.
func LibraryFileRegister(prgrm *ast.CXProgram) {}

// LibraryProcessRegister registers nothing in a cx built with nocxos.
func LibraryProcessRegister(prgrm *ast.CXProgram) {}
