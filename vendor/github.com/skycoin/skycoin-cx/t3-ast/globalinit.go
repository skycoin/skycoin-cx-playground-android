package ast

import (
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

// GlobalDeferred is a global of var g = value whose value is not a literal.
// Pass one records it, and its type is the type of the value once the types
// of the functions and of the other globals are known, as in Go
// (https://go.dev/ref/spec#Variable_declarations).
type GlobalDeferred struct {
	// GlobalDeferredDeclarator is the name, which declares the global later.
	GlobalDeferredDeclarator CXArgumentIndex
	GlobalDeferredPackage    CXPackageIndex
	GlobalDeferredFile       SourceFileGid
	// The value starts at the start position and ends before the end position.
	GlobalDeferredStartLine   int
	GlobalDeferredStartColumn int
	GlobalDeferredEndLine     int
	GlobalDeferredEndColumn   int
}

// GlobalInitializer is the run of SysInitExprs that initializes one global,
// which the init function orders by dependency, as Go does
// (https://go.dev/ref/spec#Package_initialization).
type GlobalInitializer struct {
	GlobalInitializerPackage CXPackageIndex
	GlobalInitializerName    stringpool.StringGid
	GlobalInitializerStart   int
	GlobalInitializerEnd     int
}

// CXProgramGlobalDeferredAdd records the global of declarator, whose value
// starts and ends at the positions given.
func (prgrm *CXProgram) CXProgramGlobalDeferredAdd(deferred GlobalDeferred) {
	prgrm.CxIrState.CxIrStateGlobalsDeferred = append(prgrm.CxIrState.CxIrStateGlobalsDeferred, deferred)
}

// CXProgramGlobalInitializerAdd records that SysInitExprs from start on
// initializes the global name of the package pkg.
func (prgrm *CXProgram) CXProgramGlobalInitializerAdd(pkg CXPackageIndex, name stringpool.StringGid, start int) {
	prgrm.CxIrState.CxIrStateGlobalInitializers = append(prgrm.CxIrState.CxIrStateGlobalInitializers, GlobalInitializer{
		GlobalInitializerPackage: pkg,
		GlobalInitializerName:    name,
		GlobalInitializerStart:   start,
		GlobalInitializerEnd:     len(prgrm.SysInitExprs),
	})
}
