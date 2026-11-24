package cxparsering

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t2-globals" // thin: only FoundCompileErrors, SRCPATH; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"

	"github.com/skycoin/skycoin-cx/t1-profiling" // thin: only CleanupAndExit, StartProfile, StopProfile; no depth gain
	"github.com/skycoin/skycoin-cx/t4-actions"
	"github.com/skycoin/skycoin-cx/t5-cx-runtime-printer"                // thin: only PrinterCompileErrorsPrint; no depth gain
	cxpartialparsing "github.com/skycoin/skycoin-cx/t5-cxpartialparsing" // thin: only CurrentFile, Parse, Program; no depth gain
)

/*
		ParseSourceCode takes a group of files representing CX `sourceCode` and
	 	parses it into CX program structures for `AST`.

		 ParseSourceCode performs the steps

		 step 1 :  preliminarystage

		 step 2 :  passone

		 step 2 : passtwo
*/
func ParseSourceCode(sourceCode []*os.File, fileNames []string) {
	/*
		Copy the contents of the file pointers containing the CX source
		code into sourceCodeStrings
	*/
	sourceCodeStrings := make([]string, len(sourceCode))
	for i, source := range sourceCode {
		tmp := bytes.NewBuffer(nil)
		io.Copy(tmp, source)
		sourceCodeStrings[i] = tmp.String()
	}

	if err := ParseSourceStrings(sourceCodeStrings, fileNames); err != nil {
		profiling.CleanupAndExit(types.CX_COMPILATION_ERROR)
	}
}

// ErrCompilation is returned by ParseSourceStrings when the source has
// parse or compile errors. The errors themselves are printed to stdout.
var ErrCompilation = errors.New("compilation error")

// ParseSourceStrings parses CX source held in memory into actions.AST.
// Unlike ParseSourceCode it returns on errors instead of exiting, so the
// parser can be embedded in a long running process.
func ParseSourceStrings(sourceCodeStrings []string, fileNames []string) error {
	// The compile errors print on return, and before a panic of the compiler.
	defer func() { printer.PrinterCompileErrorsPrint(actions.AST) }()
	return parseSourceStrings(sourceCodeStrings, fileNames)
}

// parseSourceStrings is ParseSourceStrings without printing the errors, which
// an imported package leaves to the program that imports it.
func parseSourceStrings(sourceCodeStrings []string, fileNames []string) error {
	cxpartialparsing.Program = actions.AST
	actions.AST.CxIrState.CxIrStateManagerGrowBoundary()
	sourceCodeStrings, fileNames = sourcesWithPackageClause(actions.AST, sourceCodeStrings, fileNames)

	/*
		We need to traverse the elements by hierarchy first add all the
		packages and structs at the same time then add globals, as these
		can be of a custom type (and it could be imported) the signatures
		of functions and methods are added in the cxpartialparsing.y pass
	*/
	// The globals of the imports, which Preliminarystage compiles, come first.
	deferredFrom := len(actions.AST.CxIrState.CxIrStateGlobalsDeferred)
	parseErrors := 0
	var packages []*ast.CXPackage
	if len(sourceCodeStrings) > 0 {
		parseErrors, packages = Preliminarystage(sourceCodeStrings, fileNames)
	}

	actions.AST = cxpartialparsing.Program
	actions.AST.CxIrState.CxIrStateManagerGrowBoundary()

	// A syntax error leaves the declarations incomplete. After other errors the
	// bodies are compiled too, and report each error pass one found once.
	if parseErrors > 0 {
		actions.AST.CxIrState.CxIrStateGlobalsDeferred = actions.AST.CxIrState.CxIrStateGlobalsDeferred[:deferredFrom]
		return ErrCompilation
	}
	for _, pkg := range packages {
		actions.PackageRedeclarationCheck(actions.AST, pkg)
		actions.ImportScopedCheck(actions.AST, pkg)
	}
	/*
		Adding global variables `OS_ARGS` to the `os` (operating system)
		package.
	*/
	if osPkg, err := actions.AST.GetPackage(actions.AST.CxIrState.CxIrStateNameOs); err == nil {
		if _, err := osPkg.GetGlobal(actions.AST, actions.AST.CxIrState.CxIrStateNameOsArgs); err != nil {
			_, arg0 := ast.MakeArgument(actions.AST, actions.AST.CxIrState.CxIrStateNameOsArgs, ast.SourceFileGidNil, -1)
			arg0.SetType(types.UNDEFINED)
			arg0.Package = ast.CXPackageIndex(osPkg.Index)

			_, arg1 := ast.MakeArgument(actions.AST, actions.AST.CxIrState.CxIrStateNameOsArgs, ast.SourceFileGidNil, -1)
			arg1.SetType(types.STR)
			arg1 = actions.DeclarationSpecifiers(arg1, []types.Pointer{0}, constants.DECL_BASIC)
			arg1 = actions.DeclarationSpecifiers(arg1, []types.Pointer{0}, constants.DECL_SLICE)
			actions.DeclareGlobalInPackage(actions.AST, osPkg, arg0, arg1, nil, false)
		}
	}

	globalsDeferredInfer(actions.AST, sourceCodeStrings, fileNames, deferredFrom)
	// The body pass meets the errors of the values of deferred globals again.
	actions.AST.CxIrState.CompileErrorList.CompileErrorListPreliminary = len(actions.AST.CxIrState.CompileErrorList.CompileErrorListRecords)

	profiling.StartProfile("4. passtwo")

	/*
	 The pass two of parsing that generates the actual output.
	*/

	for i, source := range sourceCodeStrings {
		actions.AST.CxIrState.CxIrStateManagerGrowBoundary()

		/*
			Because of an unkown reason, sometimes some CX programs
			throw an error related to a premature EOF (particularly in Windows).
			Adding a newline character solves this.
		*/

		source = source + "\n"

		// Each file selects its package with its package clause.
		actions.AST.CurrentPackage = -1
		actions.LineNo = 1

		b := bytes.NewBufferString(source)

		actions.CurrentFile = ast.SourceFileGidNil
		if len(fileNames) > 0 {
			actions.CurrentFile = actions.AST.CXProgramSourceFileIntern(fileNames[i])
		}

		profiling.StartProfile(actions.AST.CXProgramSourceFileText(actions.CurrentFile))

		actions.ImportScopedDirectivesRead(actions.CurrentFile, source)
		parseErrors += Passtwo(b)

		profiling.StopProfile(actions.AST.CXProgramSourceFileText(actions.CurrentFile))
	}

	profiling.StopProfile("4. passtwo")
	actions.AST.CxIrState.CxIrStateManagerGrowBoundary()

	if globals.FoundCompileErrors || parseErrors > 0 {
		return ErrCompilation
	}
	return nil
}

// sourcesWithPackageClause records an error at line 1 of each file that does
// not start with a package clause and returns the other files. Go does not
// parse such a file further, and its declarations would have no package.
func sourcesWithPackageClause(prgrm *ast.CXProgram, sources []string, names []string) ([]string, []string) {
	var keptSources, keptNames []string
	for i, source := range sources {
		if sourceHasPackageClause(source) {
			if keptSources != nil {
				keptSources = append(keptSources, source)
				if len(names) > i {
					keptNames = append(keptNames, names[i])
				}
			}
			continue
		}
		if keptSources == nil {
			keptSources = append(make([]string, 0, len(sources)), sources[:i]...)
			keptNames = append(make([]string, 0, len(names)), names[:min(i, len(names))]...)
		}
		file := ast.SourceFileGidNil
		if len(names) > i {
			file = prgrm.CXProgramSourceFileIntern(names[i])
		}
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeNoPackageDefined, file, 1, 1, ast.CompileErrorOperands{})
	}
	if keptSources == nil {
		return sources, names
	}
	return keptSources, keptNames
}

// sourceHasPackageClause reports whether the first token of source after
// white space and comments is the keyword package.
func sourceHasPackageClause(source string) bool {
	for {
		trimmed := strings.TrimLeft(source, " \t\r\n\ufeff")
		switch {
		case strings.HasPrefix(trimmed, "//"):
			end := strings.IndexByte(trimmed, '\n')
			if end < 0 {
				return false
			}
			source = trimmed[end:]
		case strings.HasPrefix(trimmed, "/*"):
			end := strings.Index(trimmed[2:], "*/")
			if end < 0 {
				return false
			}
			source = trimmed[end+4:]
		default:
			rest, found := strings.CutPrefix(trimmed, "package")
			return found && rest != "" && !sourceIdentByte(rest[0])
		}
	}
}

func sourceIdentByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}
