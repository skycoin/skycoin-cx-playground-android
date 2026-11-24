package cxparsering

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t2-globals" // thin: only FoundCompileErrors; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-actions"
	cxparsingcompletor "github.com/skycoin/skycoin-cx/t8-cxparsingcompletor" // thin: only Identifiers; no depth gain
)

// globalDeferredKey names a global of a package.
type globalDeferredKey struct {
	globalDeferredKeyPackage ast.CXPackageIndex
	globalDeferredKeyName    stringpool.StringGid
}

// globalsDeferredInfer gives each global pass one deferred from index from on
// the type of its value, as Go does
// (https://go.dev/ref/spec#Variable_declarations). The functions and the
// other globals are declared, and a global whose value names another deferred
// global is typed after it. A cycle among them is an initialization cycle
// (https://go.dev/ref/spec#Package_initialization).
func globalsDeferredInfer(prgrm *ast.CXProgram, sources []string, fileNames []string, from int) {
	deferred := append([]ast.GlobalDeferred(nil), prgrm.CxIrState.CxIrStateGlobalsDeferred[from:]...)
	prgrm.CxIrState.CxIrStateGlobalsDeferred = prgrm.CxIrState.CxIrStateGlobalsDeferred[:from]
	if len(deferred) == 0 {
		return
	}

	sourceOf := make(map[ast.SourceFileGid]string, len(sources))
	for i, source := range sources {
		file := ast.SourceFileGidNil
		if i < len(fileNames) {
			file = prgrm.CXProgramSourceFileIntern(fileNames[i])
		}
		sourceOf[file] = source
	}

	byName := make(map[globalDeferredKey]int, len(deferred))
	for i := range deferred {
		byName[globalDeferredKeyOf(prgrm, &deferred[i])] = i
	}
	texts := make([]string, len(deferred))
	dependencies := make([][]int, len(deferred))
	for i := range deferred {
		texts[i] = globalDeferredText(sourceOf[deferred[i].GlobalDeferredFile], &deferred[i])
		for _, name := range cxparsingcompletor.Identifiers(texts[i]) {
			key := globalDeferredKey{deferred[i].GlobalDeferredPackage, prgrm.CXProgramNameIntern(name)}
			if j, found := byName[key]; found {
				dependencies[i] = append(dependencies[i], j)
			}
		}
	}

	const (
		visitNew = iota
		visitOpen
		visitDone
	)
	visits := make([]int, len(deferred))
	failed := make([]bool, len(deferred))
	var open []int
	var visit func(i int)
	visit = func(i int) {
		visits[i] = visitOpen
		open = append(open, i)
		for _, j := range dependencies[i] {
			switch visits[j] {
			case visitNew:
				visit(j)
			case visitOpen:
				start := len(open) - 1
				for open[start] != j {
					start--
				}
				cycle := open[start:]
				globalsCycleReport(prgrm, deferred, cycle)
				for _, k := range cycle {
					failed[k] = true
				}
			}
			if failed[j] {
				failed[i] = true
			}
		}
		open = open[:len(open)-1]
		visits[i] = visitDone
		globalDeferredDeclare(prgrm, &deferred[i], texts[i], failed[i])
	}
	for i := range deferred {
		if visits[i] == visitNew {
			visit(i)
		}
	}
}

// globalDeferredKeyOf is the key of the global of deferred.
func globalDeferredKeyOf(prgrm *ast.CXProgram, deferred *ast.GlobalDeferred) globalDeferredKey {
	declarator := prgrm.GetCXArg(deferred.GlobalDeferredDeclarator)
	return globalDeferredKey{deferred.GlobalDeferredPackage, declarator.Name}
}

// globalDeferredText is the text of the value of deferred in source.
func globalDeferredText(source string, deferred *ast.GlobalDeferred) string {
	start := sourceOffset(source, deferred.GlobalDeferredStartLine, deferred.GlobalDeferredStartColumn)
	end := sourceOffset(source, deferred.GlobalDeferredEndLine, deferred.GlobalDeferredEndColumn)
	if start < 0 || end < start {
		return ""
	}
	return source[start:end]
}

// sourceOffset is the byte offset of line and column, both from 1, in source,
// or -1. A column counts bytes, as the lexers do.
func sourceOffset(source string, line int, column int) int {
	offset := 0
	for ; line > 1; line-- {
		next := strings.IndexByte(source[offset:], '\n')
		if next < 0 {
			return -1
		}
		offset += next + 1
	}
	offset += column - 1
	if offset > len(source) {
		return len(source)
	}
	return offset
}

// globalsCycleReport records the initialization cycle of the deferred globals
// cycle, each of which refers to the next and the last to the first, at the
// first.
func globalsCycleReport(prgrm *ast.CXProgram, deferred []ast.GlobalDeferred, cycle []int) {
	names := make([]string, len(cycle))
	for i, index := range cycle {
		names[i] = prgrm.CXProgramNameText(prgrm.GetCXArg(deferred[index].GlobalDeferredDeclarator).Name)
	}
	first := prgrm.GetCXArg(deferred[cycle[0]].GlobalDeferredDeclarator)
	file, line, column := ast.CXArgumentDebugPosition(prgrm, first.ArgDetails)
	prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeInitializationCycle, file, line, column, ast.CompileErrorOperands{int64(first.Name), prgrm.CXProgramCompileErrorText(globalsCycleText(names))})
}

// globalsCycleText is the chain of references of the cycle of names, as Go
// prints it.
func globalsCycleText(names []string) string {
	if len(names) == 1 {
		return names[0] + " refers to itself"
	}
	var text strings.Builder
	for i, name := range names {
		if i > 0 {
			text.WriteString(", ")
		}
		text.WriteString(name + " refers to " + names[(i+1)%len(names)])
	}
	return text.String()
}

// globalDeferredDeclare declares the global of deferred with the type of its
// value text, or with types.INVALID after an error. Its uses then report
// nothing more.
func globalDeferredDeclare(prgrm *ast.CXProgram, deferred *ast.GlobalDeferred, text string, failed bool) {
	pkg, err := prgrm.GetPackageFromArray(deferred.GlobalDeferredPackage)
	if err != nil {
		panic(err)
	}
	declarator := prgrm.GetCXArg(deferred.GlobalDeferredDeclarator)
	var spec *ast.CXArgument
	if !failed {
		var reported bool
		spec, reported = globalDeferredType(prgrm, pkg, deferred, text)
		if spec == nil && !reported && !globalsInvalidNamed(prgrm, pkg, text) {
			file, line, column := ast.CXArgumentDebugPosition(prgrm, declarator.ArgDetails)
			prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeGlobalTypeUnknown, file, line, column, ast.CompileErrorOperands{int64(declarator.Name)})
		}
	}
	currentPackage := prgrm.CurrentPackage
	prgrm.CurrentPackage = ast.CXPackageIndex(pkg.Index)
	if spec == nil {
		spec = actions.DeclarationSpecifiersBasic(prgrm, types.INVALID)
	}
	actions.DeclareGlobalInPackage(prgrm, pkg, declarator, spec, nil, false)
	prgrm.CurrentPackage = currentPackage
	prgrm.CxIrState.CxIrStateManagerGrowBoundary()
}

// globalsInvalidNamed reports whether text names a global of pkg whose type
// is invalid, whose error is recorded.
func globalsInvalidNamed(prgrm *ast.CXProgram, pkg *ast.CXPackage, text string) bool {
	for _, name := range cxparsingcompletor.Identifiers(text) {
		glbl, err := pkg.GetGlobal(prgrm, prgrm.CXProgramNameIntern(name))
		if err != nil {
			continue
		}
		if glbl.Type == ast.TYPE_ATOMIC && types.Code(glbl.Meta) == types.INVALID {
			return true
		}
	}
	return false
}

// globalDeferredType is the type spec of the value text of deferred, which it
// compiles as the value of a short declaration in a function made for it. The
// function and its expressions are dropped, and pass two compiles the value
// again in the init function. It keeps the errors of the value, which pass two
// does not record again. It returns nil and whether the value had errors when
// the value has no type a global can take.
func globalDeferredType(prgrm *ast.CXProgram, pkg *ast.CXPackage, deferred *ast.GlobalDeferred, text string) (*ast.CXArgument, bool) {
	if text == "" {
		return nil, false
	}
	functionName := globalsUnusedName(prgrm, "cxGlobalInitializer", text, func(name stringpool.StringGid) bool {
		_, found := pkg.Functions[name]
		return found
	})
	valueName := globalsUnusedName(prgrm, "cxGlobalValue", text, nil)

	errorList := &prgrm.CxIrState.CompileErrorList
	records := len(errorList.CompileErrorListRecords)
	foundCompileErrors := globals.FoundCompileErrors
	functions := len(prgrm.CXFunctions)
	atomicOps := len(prgrm.CXAtomicOps)
	currentFile, lineNo, lineStr := actions.CurrentFile, actions.LineNo, actions.LineStr
	currentPackage := prgrm.CurrentPackage
	currentFunction := pkg.CurrentFunction
	temporaryVariableCounter := actions.TemporaryVariableCounter

	actions.CurrentFile = deferred.GlobalDeferredFile
	prgrm.CurrentPackage = ast.CXPackageIndex(pkg.Index)
	fn := ast.MakeFunction(prgrm, functionName, deferred.GlobalDeferredFile, 0)
	_, fnIdx := pkg.AddFunction(prgrm, fn)
	actions.ScopeGlobalsInitializer = true
	Passtwo(bytes.NewBufferString(globalDeferredSource(prgrm, pkg, deferred, text, functionName, valueName)))
	actions.ScopeGlobalsInitializer = false

	spec := globalValueSpec(prgrm, prgrm.GetFunctionFromArray(fnIdx), valueName)
	// An error of the declaration made for the value leaves the spec unusable.
	failed := len(errorList.CompileErrorListRecords) > records
	reported := globalDeferredErrorsKeep(prgrm, deferred, records, functionName, valueName)

	delete(pkg.Functions, functionName)
	if !reported {
		globals.FoundCompileErrors = foundCompileErrors
	}
	prgrm.CXFunctions = prgrm.CXFunctions[:functions]
	prgrm.CXAtomicOps = prgrm.CXAtomicOps[:atomicOps]
	actions.CurrentFile, actions.LineNo, actions.LineStr = currentFile, lineNo, lineStr
	prgrm.CurrentPackage = currentPackage
	pkg.CurrentFunction = currentFunction
	actions.TemporaryVariableCounter = temporaryVariableCounter
	if failed {
		return nil, reported
	}
	return spec, false
}

// globalsUnusedName is a name that starts with prefix, is not in text and is
// not taken.
func globalsUnusedName(prgrm *ast.CXProgram, prefix string, text string, taken func(stringpool.StringGid) bool) stringpool.StringGid {
	name := prefix
	for i := 1; ; i++ {
		if !strings.Contains(text, name) {
			gid := prgrm.CXProgramNameIntern(name)
			if taken == nil || !taken(gid) {
				return gid
			}
		}
		name = prefix + strconv.Itoa(i)
	}
}

// globalDeferredSource is a file of the package of deferred with the function
// functionName, whose body declares valueName := text. The text keeps its
// line and column, so its uses are not taken for a declaration.
func globalDeferredSource(prgrm *ast.CXProgram, pkg *ast.CXPackage, deferred *ast.GlobalDeferred, text string, functionName stringpool.StringGid, valueName stringpool.StringGid) string {
	var file strings.Builder
	file.WriteString("package " + prgrm.CXProgramNameText(pkg.Name) + "; func " + prgrm.CXProgramNameText(functionName) + "() {")
	line := deferred.GlobalDeferredStartLine
	if line < 2 {
		line = 2
	}
	for i := 2; i < line; i++ {
		file.WriteByte('\n')
	}
	names := prgrm.CXProgramNameText(valueName)
	if deferred.GlobalDeferredResults > 1 {
		// var a, b = f() takes one of the results of f.
		results := make([]string, deferred.GlobalDeferredResults)
		for i := range results {
			results[i] = names + "Other" + strconv.Itoa(i)
		}
		results[deferred.GlobalDeferredResult] = names
		names = strings.Join(results, ", ")
	}
	file.WriteString(" " + names + " :=\n")
	file.WriteString(strings.Repeat(" ", deferred.GlobalDeferredStartColumn-1))
	file.WriteString(text)
	file.WriteString("\n}\n")
	return file.String()
}

// globalValueSpec is the type spec of the local valueName that fn declares,
// or nil when it has no type a global can take. A local in a block has the
// name n:valueName.
func globalValueSpec(prgrm *ast.CXProgram, fn *ast.CXFunction, valueName stringpool.StringGid) *ast.CXArgument {
	name := prgrm.CXProgramNameText(valueName)
	for _, expr := range fn.Expressions {
		if expr.Type == ast.CX_LINE {
			continue
		}
		op := &prgrm.CXAtomicOps[expr.Index]
		for _, output := range op.GetOutputs(prgrm) {
			sig := prgrm.GetCXTypeSignatureFromArray(output)
			if text := prgrm.CXProgramNameText(sig.Name); text == name || strings.HasSuffix(text, ":"+name) {
				return globalSpecOfSignature(prgrm, sig)
			}
		}
	}
	return nil
}

// globalSpecOfSignature is the type spec of the type of sig, built as the
// parser builds the spec of the type written out, or nil for a type that
// cannot be written, such as an invalid one.
func globalSpecOfSignature(prgrm *ast.CXProgram, sig *ast.CXTypeSignature) *ast.CXArgument {
	reference := sig.PassBy == constants.PASSBY_REFERENCE
	switch sig.Type {
	case ast.TYPE_ATOMIC:
		return globalSpecPointer(globalSpecBasic(prgrm, types.Code(sig.Meta)), reference)
	case ast.TYPE_POINTER_ATOMIC:
		return globalSpecPointer(globalSpecBasic(prgrm, types.Code(sig.Meta)), !sig.IsDeref)
	case ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC:
		array := prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta)
		spec := globalSpecBasic(prgrm, types.Code(array.Meta))
		if spec == nil || len(array.Lengths) <= len(array.Indexes) {
			return nil
		}
		spec = actions.DeclarationSpecifiers(spec, append([]types.Pointer(nil), array.Lengths[len(array.Indexes):]...), constants.DECL_ARRAY)
		if sig.Type == ast.TYPE_POINTER_ARRAY_ATOMIC {
			reference = !sig.IsDeref
		}
		return globalSpecPointer(spec, reference)
	case ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		array := prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta)
		spec := globalSpecBasic(prgrm, types.Code(array.Meta))
		if spec == nil || len(array.Lengths) <= len(array.Indexes) {
			return nil
		}
		for range array.Lengths[len(array.Indexes):] {
			spec = actions.DeclarationSpecifiers(spec, []types.Pointer{0}, constants.DECL_SLICE)
		}
		if sig.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
			reference = !sig.IsDeref
		}
		return globalSpecPointer(spec, reference)
	case ast.TYPE_STRUCT, ast.TYPE_POINTER_STRUCT:
		details := prgrm.GetCXTypeSignatureStructFromArray(sig.Meta)
		if len(details.Fields) > 0 || details.StructType == nil {
			return nil
		}
		if sig.Type == ast.TYPE_POINTER_STRUCT {
			reference = !sig.IsDeref
		}
		return globalSpecPointer(globalSpecStruct(prgrm, details.StructType), reference)
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		return globalSpecOfArgument(prgrm, prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta)))
	}
	return nil
}

// globalSpecOfArgument is the type spec of the type of the declared local
// arg, or nil.
func globalSpecOfArgument(prgrm *ast.CXProgram, arg *ast.CXArgument) *ast.CXArgument {
	if len(arg.Fields) > 0 || len(arg.Indexes) > 0 || len(arg.DeclarationSpecifiers) == 0 {
		return nil
	}
	var spec *ast.CXArgument
	switch {
	case arg.StructType != nil:
		// A slice of structs has a basic first specifier.
		spec = globalSpecStruct(prgrm, arg.StructType)
	case arg.DeclarationSpecifiers[0] == constants.DECL_BASIC:
		code := arg.Type
		if code == types.POINTER {
			code = arg.PointerTargetType
		}
		spec = globalSpecBasic(prgrm, code)
	}
	if spec == nil {
		return nil
	}
	arrays := 0
	for _, decl := range arg.DeclarationSpecifiers[1:] {
		switch decl {
		case constants.DECL_ARRAY:
			arrays++
			spec = actions.DeclarationSpecifiers(spec, []types.Pointer{0}, constants.DECL_ARRAY)
		case constants.DECL_SLICE:
			spec = actions.DeclarationSpecifiers(spec, []types.Pointer{0}, constants.DECL_SLICE)
		case constants.DECL_POINTER:
			spec = actions.DeclarationSpecifiers(spec, nil, constants.DECL_POINTER)
		default:
			return nil
		}
	}
	if len(arg.Lengths) != len(spec.Lengths) {
		return nil
	}
	spec.Lengths = append([]types.Pointer(nil), arg.Lengths...)
	return spec
}

// globalSpecBasic is the spec of the basic type code, or nil for a type that
// cannot be written.
func globalSpecBasic(prgrm *ast.CXProgram, code types.Code) *ast.CXArgument {
	switch code {
	case types.UNDEFINED, types.INVALID, types.IDENTIFIER, types.POINTER, types.STRUCT, types.FUNC:
		return nil
	}
	return actions.DeclarationSpecifiersBasic(prgrm, code)
}

// globalSpecStruct is the spec of the struct strct, as DeclarationSpecifiersStruct builds it.
func globalSpecStruct(prgrm *ast.CXProgram, strct *ast.CXStruct) *ast.CXArgument {
	if strct == nil {
		return nil
	}
	_, arg := ast.MakeArgument(prgrm, stringpool.StringGidNil, actions.CurrentFile, actions.LineNo)
	arg.Type = types.STRUCT
	arg.StructType = strct
	arg.Size = strct.GetStructSize(prgrm)
	arg.Package = prgrm.CurrentPackage
	arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_STRUCT)
	return arg
}

// globalSpecPointer is spec, or a pointer to it when pointer is true.
func globalSpecPointer(spec *ast.CXArgument, pointer bool) *ast.CXArgument {
	if spec == nil || !pointer {
		return spec
	}
	return actions.DeclarationSpecifiers(spec, nil, constants.DECL_POINTER)
}

// globalDeferredErrorsKeep drops the errors recorded from index records on
// that are not in the value of deferred or that name the function or the
// local made for it, and reports whether any is left.
func globalDeferredErrorsKeep(prgrm *ast.CXProgram, deferred *ast.GlobalDeferred, records int, functionName stringpool.StringGid, valueName stringpool.StringGid) bool {
	errorList := &prgrm.CxIrState.CompileErrorList
	kept := errorList.CompileErrorListRecords[:records]
	countMismatch := false
	for _, record := range errorList.CompileErrorListRecords[records:] {
		if deferred.GlobalDeferredResults > 1 && record.CompileErrorRecordCode == ast.CompileErrorCodeAssignmentCount {
			// The count of var a, b = f() is reported once, at a.
			countMismatch = true
			if deferred.GlobalDeferredResult == 0 {
				record.CompileErrorRecordFile, record.CompileErrorRecordLine, record.CompileErrorRecordColumn = ast.CXArgumentDebugPosition(prgrm, prgrm.GetCXArg(deferred.GlobalDeferredDeclarator).ArgDetails)
				kept = append(kept, record)
			}
			continue
		}
		if !globalsRecordNames(record, functionName, valueName) && record.CompileErrorRecordFile == deferred.GlobalDeferredFile && globalsPositionBefore(deferred.GlobalDeferredStartLine, deferred.GlobalDeferredStartColumn, record.CompileErrorRecordLine, record.CompileErrorRecordColumn) && globalsPositionBefore(record.CompileErrorRecordLine, record.CompileErrorRecordColumn, deferred.GlobalDeferredEndLine, deferred.GlobalDeferredEndColumn) {
			kept = append(kept, record)
		}
	}
	errorList.CompileErrorListRecords = kept
	return len(kept) > records || countMismatch
}

// globalsPositionBefore reports whether line and column are at or before
// otherLine and otherColumn. A column of 0 is anywhere on its line.
func globalsPositionBefore(line int, column int, otherLine int, otherColumn int) bool {
	return line < otherLine || line == otherLine && (column <= otherColumn || column == 0 || otherColumn == 0)
}

// globalsInitializerReference is a global that an initializer or a function
// refers to, through the functions of globalsInitializerReferenceVia.
type globalsInitializerReference struct {
	globalsInitializerReferenceName stringpool.StringGid
	globalsInitializerReferenceVia  []stringpool.StringGid
}

// globalsInitializersOrder orders the initializers of the globals of each
// package in SysInitExprs as Go does
// (https://go.dev/ref/spec#Package_initialization). A global comes after the
// globals its initializer refers to, directly or through the functions it
// calls, and the globals that are ready keep their order. A cycle of
// references is an initialization cycle, recorded here unless a global of it
// is invalid, whose error is recorded.
func globalsInitializersOrder(prgrm *ast.CXProgram) {
	initializers := prgrm.CxIrState.CxIrStateGlobalInitializers
	prgrm.CxIrState.CxIrStateGlobalInitializers = nil
	ordered := make([]ast.CXExpression, 0, len(prgrm.SysInitExprs))
	next := 0
	functions := map[ast.CXFunctionIndex][]globalsInitializerReference{}
	for start := 0; start < len(initializers); {
		end := start + 1
		for end < len(initializers) && initializers[end].GlobalInitializerPackage == initializers[start].GlobalInitializerPackage {
			end++
		}
		run := initializers[start:end]
		ordered = append(ordered, prgrm.SysInitExprs[next:run[0].GlobalInitializerStart]...)
		for _, index := range globalsInitializersRunOrder(prgrm, run, functions) {
			ordered = append(ordered, prgrm.SysInitExprs[run[index].GlobalInitializerStart:run[index].GlobalInitializerEnd]...)
		}
		next = run[len(run)-1].GlobalInitializerEnd
		start = end
	}
	prgrm.SysInitExprs = append(ordered, prgrm.SysInitExprs[next:]...)
}

// globalsInitializersRunOrder returns the order of the initializers of run,
// all of one package.
func globalsInitializersRunOrder(prgrm *ast.CXProgram, run []ast.GlobalInitializer, functions map[ast.CXFunctionIndex][]globalsInitializerReference) []int {
	byName := make(map[stringpool.StringGid]int, len(run))
	for i, initializer := range run {
		byName[initializer.GlobalInitializerName] = i
		for _, other := range initializer.GlobalInitializerOthers {
			byName[other] = i
		}
	}
	pkg, err := prgrm.GetPackageFromArray(run[0].GlobalInitializerPackage)
	if err != nil {
		panic(err)
	}
	dependencies := make([][]globalsInitializerReference, len(run))
	for i, initializer := range run {
		for _, reference := range globalsInitializerReferences(prgrm, pkg, prgrm.SysInitExprs[initializer.GlobalInitializerStart:initializer.GlobalInitializerEnd], functions) {
			if _, found := byName[reference.globalsInitializerReferenceName]; found {
				dependencies[i] = append(dependencies[i], reference)
			}
		}
	}

	order := make([]int, 0, len(run))
	done := make([]bool, len(run))
	for len(order) < len(run) {
		progress := false
		for i := range run {
			if done[i] {
				continue
			}
			ready := true
			for _, reference := range dependencies[i] {
				if j := byName[reference.globalsInitializerReferenceName]; !done[j] {
					ready = false
					break
				}
			}
			if ready {
				done[i] = true
				order = append(order, i)
				progress = true
				// The earliest global that is ready goes first.
				break
			}
		}
		if !progress {
			// The globals of the cycle go next, so the others may become ready.
			for _, i := range globalsInitializersCycleReport(prgrm, pkg, run, byName, dependencies, done) {
				done[i] = true
				order = append(order, i)
			}
		}
	}
	return order
}

// globalsInitializersCycleReport records a cycle among the initializers of
// run that are not done, at its first global, unless a global of it is
// invalid, and returns the initializers of the cycle.
func globalsInitializersCycleReport(prgrm *ast.CXProgram, pkg *ast.CXPackage, run []ast.GlobalInitializer, byName map[stringpool.StringGid]int, dependencies [][]globalsInitializerReference, done []bool) []int {
	first := 0
	for done[first] {
		first++
	}
	// Following the first reference that is not done from first ends in a cycle.
	var path []int
	var via [][]stringpool.StringGid
	position := map[int]int{}
	for i := first; ; {
		if at, found := position[i]; found {
			path, via = path[at:], via[at:]
			break
		}
		position[i] = len(path)
		path = append(path, i)
		for _, reference := range dependencies[i] {
			if j := byName[reference.globalsInitializerReferenceName]; !done[j] {
				via = append(via, reference.globalsInitializerReferenceVia)
				i = j
				break
			}
		}
	}
	// The cycle starts at its earliest global, as the source reads.
	start := 0
	for k := range path {
		if path[k] < path[start] {
			start = k
		}
	}
	path = append(path[start:], path[:start]...)
	via = append(via[start:], via[:start]...)

	var chain []string
	for k, index := range path {
		name := run[index].GlobalInitializerName
		glbl, err := pkg.GetGlobal(prgrm, name)
		if err == nil && glbl.Type == ast.TYPE_ATOMIC && types.Code(glbl.Meta) == types.INVALID {
			return path
		}
		from := prgrm.CXProgramNameText(name)
		for _, function := range via[k] {
			chain = append(chain, from+" refers to "+prgrm.CXProgramNameText(function))
			from = prgrm.CXProgramNameText(function)
		}
		chain = append(chain, from+" refers to "+prgrm.CXProgramNameText(run[path[(k+1)%len(path)]].GlobalInitializerName))
	}
	text := strings.Join(chain, ", ")
	if len(chain) == 1 {
		text = prgrm.CXProgramNameText(run[path[0]].GlobalInitializerName) + " refers to itself"
	}
	name := run[path[0]].GlobalInitializerName
	glbl, err := pkg.GetGlobal(prgrm, name)
	if err != nil {
		panic(err)
	}
	file, line, column := ast.CXArgumentDebugPosition(prgrm, globalsArgDetails(prgrm, glbl))
	prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeInitializationCycle, file, line, column, ast.CompileErrorOperands{int64(name), prgrm.CXProgramCompileErrorText(text)})
	return path
}

// globalsArgDetails is the debug record of the global glbl.
func globalsArgDetails(prgrm *ast.CXProgram, glbl *ast.CXTypeSignature) ast.CXArgumentDebugGid {
	if glbl.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		return prgrm.GetCXArg(ast.CXArgumentIndex(glbl.Meta)).ArgDetails
	}
	return glbl.ArgDetails
}

// globalsInitializerReferences returns the globals of pkg that the
// expressions of an initializer, which are not compiled yet, refer to by
// name, and those that the functions they call refer to.
func globalsInitializerReferences(prgrm *ast.CXProgram, pkg *ast.CXPackage, exprs []ast.CXExpression, functions map[ast.CXFunctionIndex][]globalsInitializerReference) []globalsInitializerReference {
	var references []globalsInitializerReference
	for _, expr := range exprs {
		if expr.Type == ast.CX_LINE {
			continue
		}
		op := &prgrm.CXAtomicOps[expr.Index]
		for _, input := range op.GetInputs(prgrm) {
			globalsSignatureNames(prgrm, prgrm.GetCXTypeSignatureFromArray(input), func(name stringpool.StringGid, _ types.Pointer) {
				if _, err := pkg.GetGlobal(prgrm, name); err == nil {
					references = append(references, globalsInitializerReference{globalsInitializerReferenceName: name})
				}
			})
		}
		references = append(references, globalsFunctionReferences(prgrm, op.Operator, functions)...)
		if expr.IsMethodCall() {
			references = append(references, globalsFunctionReferences(prgrm, globalsMethodCalled(prgrm, pkg, op), functions)...)
		}
	}
	return references
}

// globalsMethodCalled returns the method that op, a method call of an
// initializer that is not compiled yet, calls on a global, as g.m() or
// g.f.m() do, or -1 when its receiver is not a global.
func globalsMethodCalled(prgrm *ast.CXProgram, pkg *ast.CXPackage, op *ast.CXAtomicOperator) ast.CXFunctionIndex {
	inputs := op.GetInputs(prgrm)
	if len(inputs) == 0 {
		return -1
	}
	sig := prgrm.GetCXTypeSignatureFromArray(inputs[0])
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return -1
	}
	receiver := prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta))
	glbl, err := pkg.GetGlobal(prgrm, receiver.Name)
	if err != nil || len(receiver.Fields) == 0 {
		return -1
	}
	strct := glbl.GetStructType(prgrm)
	for _, fieldIdx := range receiver.Fields[:len(receiver.Fields)-1] {
		if strct == nil {
			return -1
		}
		field, err := strct.GetField(prgrm, prgrm.GetCXArg(fieldIdx).Name)
		if err != nil {
			return -1
		}
		strct = field.GetStructType(prgrm)
	}
	if strct == nil {
		return -1
	}
	strctPkg, err := prgrm.GetPackageFromArray(strct.Package)
	if err != nil {
		return -1
	}
	method, err := strctPkg.GetMethod(prgrm, strct.Name, prgrm.GetCXArg(receiver.Fields[len(receiver.Fields)-1]).Name)
	if err != nil {
		return -1
	}
	return method
}

// globalsFunctionReferences returns the globals of its package that the
// compiled function fnIdx refers to, directly or through the functions it
// calls, each through fnIdx.
func globalsFunctionReferences(prgrm *ast.CXProgram, fnIdx ast.CXFunctionIndex, functions map[ast.CXFunctionIndex][]globalsInitializerReference) []globalsInitializerReference {
	fn := prgrm.GetFunctionFromArray(fnIdx)
	if fn == nil || fn.IsBuiltIn() {
		return nil
	}
	if references, found := functions[fnIdx]; found {
		return references
	}
	// A recursive call meets nil here.
	functions[fnIdx] = nil
	pkg, err := prgrm.GetPackageFromArray(fn.Package)
	if err != nil {
		return nil
	}
	var direct []globalsInitializerReference
	var called []globalsInitializerReference
	seen := map[stringpool.StringGid]bool{}
	for _, expr := range fn.Expressions {
		if expr.Type == ast.CX_LINE {
			continue
		}
		op := &prgrm.CXAtomicOps[expr.Index]
		for _, sigs := range [][]ast.CXTypeSignatureIndex{op.GetInputs(prgrm), op.GetOutputs(prgrm)} {
			for _, sigIdx := range sigs {
				globalsSignatureNames(prgrm, prgrm.GetCXTypeSignatureFromArray(sigIdx), func(name stringpool.StringGid, offset types.Pointer) {
					if seen[name] {
						return
					}
					if glbl, err := pkg.GetGlobal(prgrm, name); err == nil && globalsOffset(prgrm, glbl) == offset {
						seen[name] = true
						direct = append(direct, globalsInitializerReference{globalsInitializerReferenceName: name})
					}
				})
			}
		}
		called = append(called, globalsFunctionReferences(prgrm, op.Operator, functions)...)
	}
	references := make([]globalsInitializerReference, 0, len(direct)+len(called))
	for _, reference := range append(direct, called...) {
		via := append([]stringpool.StringGid{fn.Name}, reference.globalsInitializerReferenceVia...)
		references = append(references, globalsInitializerReference{globalsInitializerReferenceName: reference.globalsInitializerReferenceName, globalsInitializerReferenceVia: via})
	}
	functions[fnIdx] = references
	return references
}

// globalsSignatureNames calls use with the name and offset of sig and of the
// indexes it reads.
func globalsSignatureNames(prgrm *ast.CXProgram, sig *ast.CXTypeSignature, use func(stringpool.StringGid, types.Pointer)) {
	if sig.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		use(sig.Name, sig.Offset)
		return
	}
	arg := prgrm.GetCXArg(ast.CXArgumentIndex(sig.Meta))
	use(arg.Name, arg.Offset)
	for _, index := range arg.Indexes {
		globalsSignatureNames(prgrm, prgrm.GetCXTypeSignatureFromArray(index), use)
	}
}

// globalsOffset is the offset of the global glbl.
func globalsOffset(prgrm *ast.CXProgram, glbl *ast.CXTypeSignature) types.Pointer {
	if glbl.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		return prgrm.GetCXArg(ast.CXArgumentIndex(glbl.Meta)).Offset
	}
	return glbl.Offset
}

// globalsRecordNames reports whether an operand of record is one of names.
func globalsRecordNames(record ast.CompileErrorRecord, names ...stringpool.StringGid) bool {
	for _, operand := range record.CompileErrorRecordOperands {
		for _, name := range names {
			if operand == int64(name) {
				return true
			}
		}
	}
	return false
}

// globalsMethodResultsDeclare makes the result variables of the method calls
// of the initializers, as box.Area() in var area = box.Area(), locals of the
// init function fn. Pass two made them locals of the function it was in.
func globalsMethodResultsDeclare(prgrm *ast.CXProgram, fn *ast.CXFunction) {
	for _, expr := range prgrm.SysInitExprs {
		if !expr.IsMethodCall() {
			continue
		}
		for _, output := range prgrm.CXAtomicOps[expr.Index].GetOutputs(prgrm) {
			name := prgrm.GetCXTypeSignatureFromArray(output).Name
			if strings.HasPrefix(prgrm.CXProgramNameText(name), constants.METHOD_RESULT_PREFIX) && !fn.IsLocalVariable(name) {
				if err := fn.AddLocalVariableName(name); err != nil {
					panic(err)
				}
			}
		}
	}
}
