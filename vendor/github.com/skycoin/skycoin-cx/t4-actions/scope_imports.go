package actions

import (
	"slices"
	"strings"

	cxpackages "github.com/skycoin/skycoin-cx/t0-packages"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// ImportScopedName is one name of import (a, b) "pkg", at its position.
type ImportScopedName struct {
	Name   string
	Line   int
	Column int
}

// importScopedShorthand is a name the parser read as pkg.name because an
// import into scope brings it in, at its position.
type importScopedShorthand struct {
	name   stringpool.StringGid
	line   int
	column int
}

// importScopedShorthandKey is the expression of a shorthand.
type importScopedShorthandKey struct {
	index     int
	operation ast.CXOPERATION_TYPE
}

// importScopedState holds the names of the function being parsed that hide
// the imports into scope, and the shorthands it read, see #487.
type importScopedState struct {
	locals     map[stringpool.StringGid]bool
	shorthands map[importScopedShorthandKey]importScopedShorthand
}

var importScoped importScopedState

// importScopedDirectiveMaskingWarning is the text of the directive that makes
// masking in the function after it a warning, see #487 D14.
const importScopedDirectiveMaskingWarning = "//cx:masking warning"

// importScopedDirectiveKey is a line of a file.
type importScopedDirectiveKey struct {
	file ast.SourceFileGid
	line int
}

// importScopedDirectives holds the lines that hold //cx:masking warning.
var importScopedDirectives map[importScopedDirectiveKey]bool

// ImportScopedDirectivesRead records the //cx:masking warning lines of the
// source of file, which the function on the next line takes.
func ImportScopedDirectivesRead(file ast.SourceFileGid, source string) {
	if !strings.Contains(source, importScopedDirectiveMaskingWarning) {
		return
	}
	for line, text := range strings.Split(source, "\n") {
		if strings.TrimSpace(text) != importScopedDirectiveMaskingWarning {
			continue
		}
		if importScopedDirectives == nil {
			importScopedDirectives = make(map[importScopedDirectiveKey]bool)
		}
		importScopedDirectives[importScopedDirectiveKey{file, line + 1}] = true
	}
}

// importScopedDirectiveMasking reports whether the line before line of file
// holds //cx:masking warning.
func importScopedDirectiveMasking(file ast.SourceFileGid, line int) bool {
	return importScopedDirectives[importScopedDirectiveKey{file, line - 1}]
}

// importScopedBegin forgets the locals and shorthands of the function before.
func importScopedBegin() {
	importScoped = importScopedState{}
}

// DeclareImportScoped declares import . "path" for names nil, and import
// (names) "path" otherwise, at line and column of currentFile. A scope of
// "file" brings the names into the file only, and "" into the package.
func DeclareImportScoped(prgrm *ast.CXProgram, path string, scope string, scopeLine int, scopeColumn int, names []ImportScopedName, currentFile ast.SourceFileGid, line int, column int) {
	if scope != "" && scope != "file" {
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeImportFormInvalid, currentFile, scopeLine, scopeColumn, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText("'" + scope + "' cannot stand before an import, only file can")})
		return
	}
	DeclareImportAt(prgrm, path, currentFile, line, column)
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	source := prgrm.CXProgramNameIntern(path[strings.LastIndexByte(path, '/')+1:])
	if _, found := pkg.Imports[source]; !found {
		return
	}
	position := ast.CXProgramImportPosition{ImportFile: currentFile, ImportLine: line, ImportColumn: column}
	scoped := ast.CXProgramImportScoped{
		ImportScopedPackage:   pkg.Name,
		ImportScopedSource:    source,
		ImportScopedFile:      currentFile,
		ImportScopedFileScope: scope == "file",
		ImportScopedPosition:  position,
	}
	if names != nil {
		scoped.ImportScopedNames = []ast.CXProgramImportScopedName{}
	}
	for _, name := range names {
		nameGid := prgrm.CXProgramNameIntern(name.Name)
		if importScopedListed(scoped.ImportScopedNames, nameGid) != nil {
			prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeImportFormInvalid, currentFile, name.Line, name.Column, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText("'" + name.Name + "' is listed twice")})
			continue
		}
		scoped.ImportScopedNames = append(scoped.ImportScopedNames, ast.CXProgramImportScopedName{
			ImportScopedNameText:     nameGid,
			ImportScopedNamePosition: ast.CXProgramImportPosition{ImportFile: currentFile, ImportLine: name.Line, ImportColumn: name.Column},
		})
	}
	prgrm.ImportScoped = append(prgrm.ImportScoped, scoped)
}

// importScopedListed returns the entry of name in names, nil when it has none.
func importScopedListed(names []ast.CXProgramImportScopedName, name stringpool.StringGid) *ast.CXProgramImportScopedName {
	for index := range names {
		if names[index].ImportScopedNameText == name {
			return &names[index]
		}
	}
	return nil
}

// importScopedGlobal reports whether pkg has the global name.
func importScopedGlobal(prgrm *ast.CXProgram, pkg *ast.CXPackage, name stringpool.StringGid) bool {
	for _, fieldIdx := range pkg.Globals.Fields {
		if prgrm.GetCXTypeSignatureFromArray(fieldIdx).Name == name {
			return true
		}
	}
	return false
}

// importScopedMember reports whether name is a global, function or type of
// source, or a function or constant of a built-in package.
func importScopedMember(prgrm *ast.CXProgram, source *ast.CXPackage, name stringpool.StringGid) bool {
	if _, found := source.Functions[name]; found {
		return true
	}
	if _, found := source.Structs[name]; found {
		return true
	}
	if importScopedGlobal(prgrm, source, name) {
		return true
	}
	if !cxpackages.IsDefaultPackage(prgrm.CXProgramNameText(source.Name)) {
		return false
	}
	if _, found := prgrm.CXProgramNativeFind(&ast.NativeNames, source.Name, stringpool.StringGidNil, name); found {
		return true
	}
	_, found := prgrm.CXProgramNativeFind(&ConstNames, source.Name, stringpool.StringGidNil, name)
	return found
}

// importScopedMembers lists the names of source an import of every name
// brings in, in name order.
func importScopedMembers(prgrm *ast.CXProgram, source *ast.CXPackage) []stringpool.StringGid {
	var names []stringpool.StringGid
	for name := range source.Functions {
		names = append(names, name)
	}
	for name := range source.Structs {
		names = append(names, name)
	}
	for _, fieldIdx := range source.Globals.Fields {
		names = append(names, prgrm.GetCXTypeSignatureFromArray(fieldIdx).Name)
	}
	sourceText := prgrm.CXProgramNameText(source.Name)
	if cxpackages.IsDefaultPackage(sourceText) {
		for _, index := range []*ast.NativeNameIndex{&ast.NativeNames, &ConstNames} {
			for _, entry := range index.NativeNameIndexPackage(sourceText) {
				names = append(names, prgrm.CXProgramNameIntern(entry.NativeNameEntryText[len(sourceText)+1:]))
			}
		}
	}
	// A name the compiler made, such as *init, is no member.
	names = slices.DeleteFunc(names, func(name stringpool.StringGid) bool {
		text := prgrm.CXProgramNameText(name)
		return text == "" || !(text[0] == '_' || text[0] >= 'a' && text[0] <= 'z' || text[0] >= 'A' && text[0] <= 'Z')
	})
	slices.SortFunc(names, func(left, right stringpool.StringGid) int {
		return strings.Compare(prgrm.CXProgramNameText(left), prgrm.CXProgramNameText(right))
	})
	return slices.Compact(names)
}

// importScopedProvides returns the position at which scoped brings name
// into scope, and false when it does not.
func importScopedProvides(prgrm *ast.CXProgram, scoped *ast.CXProgramImportScoped, name stringpool.StringGid) (ast.CXProgramImportPosition, bool) {
	position := scoped.ImportScopedPosition
	if scoped.ImportScopedNames != nil {
		listed := importScopedListed(scoped.ImportScopedNames, name)
		if listed == nil {
			return position, false
		}
		position = listed.ImportScopedNamePosition
	}
	source, err := prgrm.GetPackage(scoped.ImportScopedSource)
	if err != nil || !importScopedMember(prgrm, source, name) {
		return position, false
	}
	return position, true
}

// importScopedFind returns the import into scope that brings name into file
// of pkg and its position, a file import before a package import, and nil
// when none does.
func importScopedFind(prgrm *ast.CXProgram, pkg *ast.CXPackage, file ast.SourceFileGid, name stringpool.StringGid) (*ast.CXProgramImportScoped, ast.CXProgramImportPosition) {
	for _, fileScope := range [2]bool{true, false} {
		for index := range prgrm.ImportScoped {
			scoped := &prgrm.ImportScoped[index]
			if scoped.ImportScopedPackage != pkg.Name || scoped.ImportScopedFileScope != fileScope || fileScope && scoped.ImportScopedFile != file {
				continue
			}
			if position, found := importScopedProvides(prgrm, scoped, name); found {
				return scoped, position
			}
		}
	}
	return nil, ast.CXProgramImportPosition{}
}

// importScopedDeclared reports whether name is a global, function, type or
// imported package of pkg, which an import into scope does not hide.
func importScopedDeclared(prgrm *ast.CXProgram, pkg *ast.CXPackage, name stringpool.StringGid) bool {
	if _, found := pkg.Functions[name]; found {
		return true
	}
	if _, found := pkg.Structs[name]; found {
		return true
	}
	if _, found := pkg.Imports[name]; found {
		return true
	}
	return importScopedGlobal(prgrm, pkg, name)
}

// importScopedSource returns the package an import into scope brings name of
// file of pkg from, nil when name is declared in pkg or no import brings it.
func importScopedSource(prgrm *ast.CXProgram, pkg *ast.CXPackage, file ast.SourceFileGid, name stringpool.StringGid) *ast.CXPackage {
	if len(prgrm.ImportScoped) == 0 || importScopedDeclared(prgrm, pkg, name) {
		return nil
	}
	scoped, _ := importScopedFind(prgrm, pkg, file, name)
	if scoped == nil {
		return nil
	}
	source, err := prgrm.GetPackage(scoped.ImportScopedSource)
	if err != nil {
		return nil
	}
	return source
}

// importScopedIdentifier returns the expressions of source.name for a name an
// import into scope brings in, as if the body had said source.name, and nil
// for any other name.
func importScopedIdentifier(prgrm *ast.CXProgram, name stringpool.StringGid) []ast.CXExpression {
	if len(prgrm.ImportScoped) == 0 || importScoped.locals[name] {
		return nil
	}
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		panic(err)
	}
	source := importScopedSource(prgrm, pkg, CurrentFile, name)
	if source == nil {
		return nil
	}
	list := &prgrm.CxIrState.CompileErrorList
	line, column := list.CompileErrorListTokenLine, list.CompileErrorListTokenColumn
	exprs := PostfixExpressionField(prgrm, primaryIdentifierExpressions(prgrm, source.Name), prgrm.CXProgramNameText(name))
	if len(exprs) > 0 {
		if importScoped.shorthands == nil {
			importScoped.shorthands = make(map[importScopedShorthandKey]importScopedShorthand)
		}
		last := exprs[len(exprs)-1]
		importScoped.shorthands[importScopedShorthandKey{last.Index, last.Type}] = importScopedShorthand{name: name, line: line, column: column}
	}
	return exprs
}

// ImportScopedShortDeclaration returns the target of a := as the local it
// declares. A shorthand the parser read as pkg.name is the name again, which
// hides the import for the rest of the function.
func ImportScopedShortDeclaration(prgrm *ast.CXProgram, target []ast.CXExpression) []ast.CXExpression {
	if len(target) == 0 || importScoped.shorthands == nil {
		return target
	}
	last := target[len(target)-1]
	shorthand, found := importScoped.shorthands[importScopedShorthandKey{last.Index, last.Type}]
	if !found {
		return target
	}
	if importScoped.locals == nil {
		importScoped.locals = make(map[stringpool.StringGid]bool)
	}
	importScoped.locals[shorthand.name] = true
	position := TokenPositionPush(shorthand.line, shorthand.column)
	defer TokenPositionPop(position)
	return primaryIdentifierExpressions(prgrm, shorthand.name)
}

// ImportScopedLocalsDeclare records that the function being parsed declares
// the variables names, which hide the imports into scope from here on.
func ImportScopedLocalsDeclare(prgrm *ast.CXProgram, names []*ast.CXArgument) {
	if len(prgrm.ImportScoped) == 0 {
		return
	}
	if importScoped.locals == nil {
		importScoped.locals = make(map[stringpool.StringGid]bool)
	}
	for _, name := range names {
		if name != nil {
			importScoped.locals[name.Name] = true
		}
	}
}

// importScopedStruct returns the struct name an import into scope brings into
// file of pkg, nil when none does.
func importScopedStruct(prgrm *ast.CXProgram, pkg *ast.CXPackage, file ast.SourceFileGid, name stringpool.StringGid) *ast.CXStruct {
	source := importScopedSource(prgrm, pkg, file, name)
	if source == nil {
		return nil
	}
	strct, err := prgrm.GetStruct(name, source.Name)
	if err != nil {
		return nil
	}
	return strct
}

// importScopedGlobalPackage returns the package an import into scope brings
// the global name of the current file from, nil when none does.
func importScopedGlobalPackage(prgrm *ast.CXProgram, pkg *ast.CXPackage, name stringpool.StringGid) *ast.CXPackage {
	source := importScopedSource(prgrm, pkg, CurrentFile, name)
	if source == nil || !importScopedGlobal(prgrm, source, name) {
		return nil
	}
	return source
}

// importScopedMaskCheck reports a local, parameter or result of the function
// being checked that has a name an import into scope brings in, a warning
// under //cx:masking warning.
func importScopedMaskCheck(prgrm *ast.CXProgram, pkg *ast.CXPackage, typeSig *ast.CXTypeSignature, name stringpool.StringGid) {
	if len(prgrm.ImportScoped) == 0 {
		return
	}
	file, _, _ := ast.CXArgumentDebugPosition(prgrm, typeSig.ArgDetails)
	scoped, position := importScopedFind(prgrm, pkg, file, name)
	if scoped == nil {
		return
	}
	code := ast.CompileErrorCodeImportMasking
	if scopeGlobals.maskingWarning {
		code = ast.CompileErrorCodeImportMaskingWarning
	}
	prgrm.CXProgramCompileErrorAddDebug(code, typeSig.ArgDetails, ast.CompileErrorOperands{int64(name), int64(position.ImportFile), int64(position.ImportLine), int64(position.ImportColumn)})
}

// importScopedMaskReport records that the declaration of name at file, line
// and column masks the import into scope at position.
func importScopedMaskReport(prgrm *ast.CXProgram, name stringpool.StringGid, file ast.SourceFileGid, line int, column int, position ast.CXProgramImportPosition) {
	prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeImportMasking, file, line, column, ast.CompileErrorOperands{int64(name), int64(position.ImportFile), int64(position.ImportLine), int64(position.ImportColumn)})
}

// ImportScopedCheck reports the names the imports into scope of pkg list that
// the package does not have, and every name they bring in that a global,
// function, type or imported package of pkg, or an earlier import into the
// same scope, has too. It runs once the declarations of pkg are known.
func ImportScopedCheck(prgrm *ast.CXProgram, pkg *ast.CXPackage) {
	for index := range prgrm.ImportScoped {
		scoped := &prgrm.ImportScoped[index]
		if scoped.ImportScopedPackage != pkg.Name {
			continue
		}
		source, err := prgrm.GetPackage(scoped.ImportScopedSource)
		if err != nil || prgrm.CXProgramImportFailed(source.Name) {
			continue
		}
		var names []stringpool.StringGid
		if scoped.ImportScopedNames == nil {
			names = importScopedMembers(prgrm, source)
		}
		for _, listed := range scoped.ImportScopedNames {
			if !importScopedMember(prgrm, source, listed.ImportScopedNameText) {
				position := listed.ImportScopedNamePosition
				prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeImportNameUnknown, position.ImportFile, position.ImportLine, position.ImportColumn, ast.CompileErrorOperands{int64(listed.ImportScopedNameText), int64(source.Name)})
				continue
			}
			names = append(names, listed.ImportScopedNameText)
		}
		for _, name := range names {
			position, _ := importScopedProvides(prgrm, scoped, name)
			importScopedCheckName(prgrm, pkg, index, name, position)
		}
	}
}

// importScopedCheckName reports what name, which import number index of pkg
// brings in at position, masks.
func importScopedCheckName(prgrm *ast.CXProgram, pkg *ast.CXPackage, index int, name stringpool.StringGid, position ast.CXProgramImportPosition) {
	if glbl, err := pkg.GetGlobal(prgrm, name); err == nil {
		file, line, column := ast.CXArgumentDebugPosition(prgrm, glbl.ArgDetails)
		importScopedMaskReport(prgrm, name, file, line, column, position)
		return
	}
	if fnIdx, found := pkg.Functions[name]; found {
		fn := prgrm.GetFunctionFromArray(fnIdx)
		importScopedMaskReport(prgrm, name, fn.FileName, fn.FileLine, fn.FileColumn, position)
		return
	}
	if strctIdx, found := pkg.Structs[name]; found {
		strct := prgrm.GetStructFromArray(strctIdx)
		importScopedMaskReport(prgrm, name, strct.FileName, strct.FileLine, strct.FileColumn, position)
		return
	}
	if _, found := pkg.Imports[name]; found {
		imported := prgrm.ImportPositions[ast.CXProgramImportKey{ImportPackage: pkg.Name, ImportName: name}]
		importScopedMaskReport(prgrm, name, imported.ImportFile, imported.ImportLine, imported.ImportColumn, position)
		return
	}
	scoped := &prgrm.ImportScoped[index]
	for earlierIndex := range prgrm.ImportScoped[:index] {
		earlier := &prgrm.ImportScoped[earlierIndex]
		if earlier.ImportScopedPackage != pkg.Name || earlier.ImportScopedSource == scoped.ImportScopedSource {
			continue
		}
		if earlier.ImportScopedFileScope && scoped.ImportScopedFileScope && earlier.ImportScopedFile != scoped.ImportScopedFile {
			continue
		}
		if earlierPosition, found := importScopedProvides(prgrm, earlier, name); found {
			importScopedMaskReport(prgrm, name, position.ImportFile, position.ImportLine, position.ImportColumn, earlierPosition)
			return
		}
	}
}

// importScopedNamesPending holds the names of import (a, b) "pkg" the
// parser has read, so its value stack holds no list of names.
var importScopedNamesPending []ImportScopedName

// ImportScopedNamesAdd records a name of import (a, b) "pkg" at its position.
func ImportScopedNamesAdd(name string, line int, column int) {
	importScopedNamesPending = append(importScopedNamesPending, ImportScopedName{Name: name, Line: line, Column: column})
}

// ImportScopedNamesTake returns the names ImportScopedNamesAdd recorded and
// forgets them.
func ImportScopedNamesTake() []ImportScopedName {
	names := importScopedNamesPending
	importScopedNamesPending = nil
	return names
}
