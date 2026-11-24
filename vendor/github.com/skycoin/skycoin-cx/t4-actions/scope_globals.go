package actions

import (
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// ScopeGlobalName is one name of a global statement, `counter` or
// `config.Limit`, at its position. Package is StringGidNil for the own
// package.
type ScopeGlobalName struct {
	Package stringpool.StringGid
	Name    stringpool.StringGid
	Line    int
	Column  int
}

// scopeGlobalDeclaration is a global a function declared, the name as
// written with its position, and whether the body used it.
type scopeGlobalDeclaration struct {
	key  SymbolsDataKey
	name ScopeGlobalName
	used bool
}

// scopeGlobalNameText returns the name as written, for an error.
func scopeGlobalNameText(prgrm *ast.CXProgram, name ScopeGlobalName) string {
	if name.Package == stringpool.StringGidNil {
		return prgrm.CXProgramNameText(name.Name)
	}
	return prgrm.CXProgramNameText(name.Package) + "." + prgrm.CXProgramNameText(name.Name)
}

// scopeGlobalsState holds the global statements of the function being
// parsed, from its header to the end of FunctionDeclaration, see #486.
type scopeGlobalsState struct {
	function      ast.CXFunctionIndex
	active        bool
	checking      bool
	statementSeen bool
	// maskingWarning is set by //cx:masking warning on the line before the
	// function, see #487.
	maskingWarning bool
	declarations   []scopeGlobalDeclaration
	reported       map[SymbolsDataKey]bool
}

var scopeGlobals scopeGlobalsState

// ScopeGlobalsInitializer is set while the compiler parses the value of a
// global as the body of a function of its own, which declares no globals.
var ScopeGlobalsInitializer bool

// ScopeGlobalsBegin starts the global statements of the function fnIdx,
// whose header the parser just read.
func ScopeGlobalsBegin(fnIdx ast.CXFunctionIndex) {
	fn := AST.GetFunctionFromArray(fnIdx)
	scopeGlobals = scopeGlobalsState{function: fnIdx, active: !ScopeGlobalsInitializer, maskingWarning: importScopedDirectiveMasking(fn.FileName, fn.FileLine)}
	importScopedBegin()
}

// ScopeGlobalsStatementSeen records that the function body has a statement,
// after which a global statement is out of place.
func ScopeGlobalsStatementSeen() {
	scopeGlobals.statementSeen = true
}

// ScopeGlobalsDeclare records `global names` at line and column, braceDepth
// braces deep, which must be among the first statements of the body.
func ScopeGlobalsDeclare(prgrm *ast.CXProgram, names []ScopeGlobalName, braceDepth int, line int, column int) {
	if !scopeGlobals.active || scopeGlobals.statementSeen || braceDepth != 1 {
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeGlobalDeclarationPlacement, CurrentFile, line, column, ast.CompileErrorOperands{})
		return
	}
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	for _, name := range names {
		target := pkg
		if name.Package != stringpool.StringGidNil {
			imported, err := pkg.GetImport(prgrm, name.Package)
			if err != nil {
				prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeGlobalDeclarationPackageMissing, CurrentFile, name.Line, name.Column, ast.CompileErrorOperands{int64(name.Package), prgrm.CXProgramCompileErrorText(scopeGlobalNameText(prgrm, name))})
				continue
			}
			target = imported
		}
		// A global an import into scope brings in is declared by its name alone.
		if name.Package == stringpool.StringGidNil {
			if source := importScopedGlobalPackage(prgrm, pkg, name.Name); source != nil {
				target = source
			}
		}
		if _, err := target.GetGlobal(prgrm, name.Name); err != nil {
			if !prgrm.CXProgramImportFailed(target.Name) {
				prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeGlobalDeclarationMissing, CurrentFile, name.Line, name.Column, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(scopeGlobalNameText(prgrm, name)), int64(target.Name)})
			}
			continue
		}
		key := SymbolsDataKey{SymbolsDataKeyPackage: target.Name, SymbolsDataKeyName: name.Name}
		if scopeGlobalsDeclared(key) != nil {
			prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeGlobalDeclarationRepeated, CurrentFile, name.Line, name.Column, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(scopeGlobalNameText(prgrm, name))})
			continue
		}
		scopeGlobals.declarations = append(scopeGlobals.declarations, scopeGlobalDeclaration{key: key, name: name})
	}
}

// scopeGlobalsDeclared returns the declaration of the global key in the
// function being parsed, nil when it has none.
func scopeGlobalsDeclared(key SymbolsDataKey) *scopeGlobalDeclaration {
	for index := range scopeGlobals.declarations {
		if scopeGlobals.declarations[index].key == key {
			return &scopeGlobals.declarations[index]
		}
	}
	return nil
}

// scopeGlobalsCheckBegin starts checking the uses of globals in fnIdx, when
// the parser read its header.
func scopeGlobalsCheckBegin(fnIdx ast.CXFunctionIndex) {
	scopeGlobals.checking = scopeGlobals.active && scopeGlobals.function == fnIdx
}

// scopeGlobalsUse records that the function uses the global ident of symPkg
// at details. A global the function did not declare is a compile error once.
func scopeGlobalsUse(prgrm *ast.CXProgram, symPkg *ast.CXPackage, ident stringpool.StringGid, details ast.CXArgumentDebugGid) {
	if !scopeGlobals.checking {
		return
	}
	key := SymbolsDataKey{SymbolsDataKeyPackage: symPkg.Name, SymbolsDataKeyName: ident}
	if declaration := scopeGlobalsDeclared(key); declaration != nil {
		declaration.used = true
		return
	}
	if scopeGlobals.reported[key] {
		return
	}
	if scopeGlobals.reported == nil {
		scopeGlobals.reported = make(map[SymbolsDataKey]bool)
	}
	scopeGlobals.reported[key] = true
	text := prgrm.CXProgramNameText(ident)
	if pkg, err := prgrm.GetCurrentPackage(); err == nil && pkg.Name != symPkg.Name {
		text = prgrm.CXProgramNameText(symPkg.Name) + "." + text
	}
	fn := prgrm.GetFunctionFromArray(scopeGlobals.function)
	textGid := prgrm.CXProgramCompileErrorText(text)
	prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeGlobalUndeclared, details, ast.CompileErrorOperands{int64(fn.Name), textGid, textGid})
}

// scopeGlobalsEnd reports the declared globals the function did not use and
// ends its global statements.
func scopeGlobalsEnd(prgrm *ast.CXProgram) {
	if scopeGlobals.checking {
		fn := prgrm.GetFunctionFromArray(scopeGlobals.function)
		for _, declaration := range scopeGlobals.declarations {
			if !declaration.used {
				prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeGlobalDeclaredUnused, fn.FileName, declaration.name.Line, declaration.name.Column, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(scopeGlobalNameText(prgrm, declaration.name)), int64(fn.Name)})
			}
		}
		scopeGlobals = scopeGlobalsState{}
	}
	importScopedBegin()
}

// scopeShadowCheck reports a local, parameter or result named like a global,
// import, function or type of pkg, or a variable of an outer scope (#486).
func scopeShadowCheck(prgrm *ast.CXProgram, symbolsData *SymbolsData, pkg *ast.CXPackage, typeSig *ast.CXTypeSignature) {
	if !scopeGlobals.checking {
		return
	}
	name := StripNameNumber(prgrm, typeSig.Name)
	operands := func(file ast.SourceFileGid, line int, column int) ast.CompileErrorOperands {
		return ast.CompileErrorOperands{int64(name), int64(file), int64(line), int64(column)}
	}
	if glbl, err := pkg.GetGlobal(prgrm, name); err == nil {
		file, line, column := ast.CXArgumentDebugPosition(prgrm, glbl.ArgDetails)
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeShadowGlobal, typeSig.ArgDetails, operands(file, line, column))
		return
	}
	if _, found := pkg.Imports[name]; found {
		position := prgrm.ImportPositions[ast.CXProgramImportKey{ImportPackage: pkg.Name, ImportName: name}]
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeShadowImport, typeSig.ArgDetails, operands(position.ImportFile, position.ImportLine, position.ImportColumn))
		return
	}
	if fnIdx, found := pkg.Functions[name]; found {
		fn := prgrm.GetFunctionFromArray(fnIdx)
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeShadowFunction, typeSig.ArgDetails, operands(fn.FileName, fn.FileLine, fn.FileColumn))
		return
	}
	if strctIdx, found := pkg.Structs[name]; found {
		strct := prgrm.GetStructFromArray(strctIdx)
		prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeShadowType, typeSig.ArgDetails, operands(strct.FileName, strct.FileLine, strct.FileColumn))
		return
	}
	key := SymbolsDataKey{SymbolsDataKeyPackage: pkg.Name, SymbolsDataKeyName: name}
	for scope := len(symbolsData.symbolsIndex) - 2; scope >= 0; scope-- {
		if symIdx, found := symbolsData.symbolsIndex[scope][key]; found {
			outer := prgrm.GetCXTypeSignatureFromArray(symbolsData.symbols[symIdx])
			file, line, column := ast.CXArgumentDebugPosition(prgrm, outer.ArgDetails)
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeShadowVariable, typeSig.ArgDetails, operands(file, line, column))
			return
		}
	}
	importScopedMaskCheck(prgrm, pkg, typeSig, name)
}
