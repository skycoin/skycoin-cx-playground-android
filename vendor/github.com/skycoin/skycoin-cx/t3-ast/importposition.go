package ast

import stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"

// CXProgramImportKey names the import of package ImportName by package
// ImportPackage.
type CXProgramImportKey struct {
	ImportPackage stringpool.StringGid
	ImportName    stringpool.StringGid
}

// CXProgramImportPosition is the file, line and column of an import
// declaration. A column of 0 is unknown.
type CXProgramImportPosition struct {
	ImportFile   SourceFileGid
	ImportLine   int
	ImportColumn int
}

// CXProgramImportPositionAdd records the first position at which package pkg
// imports package name.
func (prgrm *CXProgram) CXProgramImportPositionAdd(pkg stringpool.StringGid, name stringpool.StringGid, position CXProgramImportPosition) {
	key := CXProgramImportKey{ImportPackage: pkg, ImportName: name}
	if _, found := prgrm.ImportPositions[key]; found {
		return
	}
	if prgrm.ImportPositions == nil {
		prgrm.ImportPositions = make(map[CXProgramImportKey]CXProgramImportPosition)
	}
	prgrm.ImportPositions[key] = position
}

// CXProgramImportScoped is an import into scope of #487. Package
// ImportScopedPackage may name the members of package ImportScopedSource
// without the package name, in the file ImportScopedFile only when
// ImportScopedFileScope is set. ImportScopedNames lists the members it
// imports, and nil imports every member.
type CXProgramImportScoped struct {
	ImportScopedPackage   stringpool.StringGid
	ImportScopedSource    stringpool.StringGid
	ImportScopedFile      SourceFileGid
	ImportScopedFileScope bool
	ImportScopedNames     []CXProgramImportScopedName
	ImportScopedPosition  CXProgramImportPosition
}

// CXProgramImportScopedName is one name an import into scope lists, at its
// position.
type CXProgramImportScopedName struct {
	ImportScopedNameText     stringpool.StringGid
	ImportScopedNamePosition CXProgramImportPosition
}
