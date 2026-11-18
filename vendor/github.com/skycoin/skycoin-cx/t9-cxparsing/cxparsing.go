package cxparsering

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t2-globals"
	"github.com/skycoin/skycoin-cx/t3-ast"

	"github.com/skycoin/skycoin-cx/t1-profiling"
	"github.com/skycoin/skycoin-cx/t4-actions"
	cxpartialparsing "github.com/skycoin/skycoin-cx/t5-cxpartialparsing"
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
		profiling.CleanupAndExit(constants.CX_COMPILATION_ERROR)
	}
}

// ErrCompilation is returned by ParseSourceStrings when the source has
// parse or compile errors. The errors themselves are printed to stdout.
var ErrCompilation = errors.New("compilation error")

// ParseSourceStrings parses CX source held in memory into actions.AST.
// Unlike ParseSourceCode it returns on errors instead of exiting, so the
// parser can be embedded in a long running process.
func ParseSourceStrings(sourceCodeStrings []string, fileNames []string) error {
	cxpartialparsing.Program = actions.AST

	/*
		We need to traverse the elements by hierarchy first add all the
		packages and structs at the same time then add globals, as these
		can be of a custom type (and it could be imported) the signatures
		of functions and methods are added in the cxpartialparsing.y pass
	*/
	parseErrors := 0
	if len(sourceCodeStrings) > 0 {
		parseErrors = Preliminarystage(sourceCodeStrings, fileNames)
	}

	actions.AST = cxpartialparsing.Program

	if globals.FoundCompileErrors || parseErrors > 0 {
		return ErrCompilation
	}

	/*
		Adding global variables `OS_ARGS` to the `os` (operating system)
		package.
	*/
	if osPkg, err := actions.AST.GetPackage(constants.OS_PKG); err == nil {
		if _, err := osPkg.GetGlobal(actions.AST, constants.OS_ARGS); err != nil {
			arg0 := ast.MakeArgument(constants.OS_ARGS, "", -1).SetType(types.UNDEFINED)
			arg0.Package = ast.CXPackageIndex(osPkg.Index)

			arg1 := ast.MakeArgument(constants.OS_ARGS, "", -1).SetType(types.STR)
			arg1 = actions.DeclarationSpecifiers(arg1, []types.Pointer{0}, constants.DECL_BASIC)
			arg1 = actions.DeclarationSpecifiers(arg1, []types.Pointer{0}, constants.DECL_SLICE)
			actions.DeclareGlobalInPackage(actions.AST, osPkg, arg0, arg1, nil, false)
		}
	}

	profiling.StartProfile("4. passtwo")

	/*
	 The pass two of parsing that generates the actual output.
	*/

	for i, source := range sourceCodeStrings {

		/*
			Because of an unkown reason, sometimes some CX programs
			throw an error related to a premature EOF (particularly in Windows).
			Adding a newline character solves this.
		*/

		source = source + "\n"

		actions.LineNo = 1

		b := bytes.NewBufferString(source)

		if len(fileNames) > 0 {
			actions.CurrentFile = fileNames[i]
		}

		profiling.StartProfile(actions.CurrentFile)

		parseErrors += Passtwo(b)

		profiling.StopProfile(actions.CurrentFile)
	}

	profiling.StopProfile("4. passtwo")

	if globals.FoundCompileErrors || parseErrors > 0 {
		return ErrCompilation
	}
	return nil
}
