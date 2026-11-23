package cxparsering

import (
	"bufio"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	cxpackages "github.com/skycoin/skycoin-cx/t0-packages" // thin: only IsBuiltinImport; no depth gain
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t0-util"
	"github.com/skycoin/skycoin-cx/t1-profiling" // thin: only CleanupAndExit, StartProfile, StopProfile; no depth gain
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t2-globals" // thin: only FoundCompileErrors, SRCPATH; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-actions"
	cxpartialparsing "github.com/skycoin/skycoin-cx/t5-cxpartialparsing" // thin: only CurrentFile, Parse, Program; no depth gain
)

// preliminarystage performs a first pass for the CX cxgo. Globals, packages and
// custom types are added to `cxpartialparsing.Program`.
// It returns the number of syntax errors and the packages the files declare.
func Preliminarystage(srcStrs, srcNames []string) (int, []*ast.CXPackage) {
	var prePkg *ast.CXPackage
	var packages []*ast.CXPackage
	parseErrors := 0

	reMultiCommentOpen := regexp.MustCompile(`/\*`)
	reMultiCommentClose := regexp.MustCompile(`\*/`)
	reComment := regexp.MustCompile("//")

	rePkg := regexp.MustCompile("package")
	rePkgName := regexp.MustCompile(`(^|[\s])package\s+([_a-zA-Z][_a-zA-Z0-9]*)`)
	reStrct := regexp.MustCompile("type")
	reStrctName := regexp.MustCompile(`(^|[\s])type\s+([_a-zA-Z][_a-zA-Z0-9]*)?\s`)

	reGlbl := regexp.MustCompile("var")
	reGlblName := regexp.MustCompile(`(^|[\s])var\s([_a-zA-Z][_a-zA-Z0-9]*)`)

	reBodyOpen := regexp.MustCompile("{")
	reBodyClose := regexp.MustCompile("}")

	reImp := regexp.MustCompile("import")
	reImpName := regexp.MustCompile(`(^|[\s])import\s+"([_a-zA-Z][_a-zA-Z0-9/-]*)"`)

	profiling.StartProfile("1. packages/structs")
	// 1. Identify all the packages and structs
	for srcI, srcStr := range srcStrs {
		actions.AST.CxIrState.CxIrStateManagerGrowBoundary()
		srcName := srcNames[srcI]
		profiling.StartProfile(srcName)

		reader := strings.NewReader(srcStr)
		scanner := bufio.NewScanner(reader)
		var commentedCode bool
		var lineno = 0
		for scanner.Scan() {
			line := scanner.Bytes()
			lineno++

			// Identify whether we are in a comment or not.
			commentLoc := reComment.FindIndex(line)
			multiCommentOpenLoc := reMultiCommentOpen.FindIndex(line)
			multiCommentCloseLoc := reMultiCommentClose.FindIndex(line)
			if commentedCode && multiCommentCloseLoc != nil {
				commentedCode = false
			}
			if commentedCode {
				continue
			}
			if multiCommentOpenLoc != nil && !commentedCode && multiCommentCloseLoc == nil {
				commentedCode = true
				continue
			}

			// At this point we know that we are *not* in a comment
			// 1-a. Identify all the packages
			if loc := rePkg.FindIndex(line); loc != nil {
				if (commentLoc != nil && commentLoc[0] < loc[0]) ||
					(multiCommentOpenLoc != nil && multiCommentOpenLoc[0] < loc[0]) ||
					(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] > loc[0]) {
					// then it's commented out
					continue
				}

				if match := rePkgName.FindStringSubmatch(string(line)); match != nil {
					if pkg, err := cxpartialparsing.Program.GetPackage(cxpartialparsing.Program.CXProgramNameIntern(match[len(match)-1])); err != nil {
						// then it hasn't been added
						// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
						// Now a constructor call, site S1620.
						newPkg := ast.MakePackage(cxpartialparsing.Program.CXProgramNameIntern(match[len(match)-1]))
						// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
						// Now an arena insert, site S1621.
						pkgIdx := cxpartialparsing.Program.AddPackage(newPkg)
						// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
						// Now an arena getter, site S1622.
						newPkg, err = cxpartialparsing.Program.GetPackageFromArray(pkgIdx)
						if err != nil {
							panic(err)
						}
						prePkg = newPkg
					} else {
						prePkg = pkg
					}
					if !slices.Contains(packages, prePkg) {
						packages = append(packages, prePkg)
					}
				}
			}

			// 1-b. Identify all the structs
			if loc := reStrct.FindIndex(line); loc != nil {
				if (commentLoc != nil && commentLoc[0] < loc[0]) ||
					(multiCommentOpenLoc != nil && multiCommentOpenLoc[0] < loc[0]) ||
					(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] > loc[0]) {
					// then it's commented out
					continue
				}

				if match := reStrctName.FindStringSubmatch(string(line)); match != nil {
					// sourcesWithPackageClause dropped every file without a package clause.
					if _, err := cxpartialparsing.Program.GetStruct(cxpartialparsing.Program.CXProgramNameIntern(match[len(match)-1]), prePkg.Name); err != nil {
						// then it hasn't been added
						// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
						// Now a constructor call, site S1623.
						strct := ast.MakeStruct(cxpartialparsing.Program.CXProgramNameIntern(match[len(match)-1]))
						prePkg.AddStruct(cxpartialparsing.Program, strct)
					}
				}
			}
		}
		profiling.StopProfile(srcName)
	} // for range srcStrs
	profiling.StopProfile("1. packages/structs")

	profiling.StartProfile("2. globals")
	// 2. Identify all global variables
	//    We also identify packages again, so we know to what
	//    package we're going to add the variable declaration to.
	for i, source := range srcStrs {
		actions.AST.CxIrState.CxIrStateManagerGrowBoundary()
		profiling.StartProfile(srcNames[i])
		// inBlock needs to be 0 to guarantee that we're in the global scope
		var inBlock int
		var commentedCode bool

		var lineno int
		scanner := bufio.NewScanner(strings.NewReader(source))
		for scanner.Scan() {
			line := scanner.Bytes()
			lineno++

			// we need to ignore function bodies
			// it'll also ignore struct declaration's bodies, but this doesn't matter
			commentLoc := reComment.FindIndex(line)

			multiCommentOpenLoc := reMultiCommentOpen.FindIndex(line)
			multiCommentCloseLoc := reMultiCommentClose.FindIndex(line)

			if commentedCode && multiCommentCloseLoc != nil {
				commentedCode = false
			}

			if commentedCode {
				continue
			}

			if multiCommentOpenLoc != nil && !commentedCode && multiCommentCloseLoc == nil {
				commentedCode = true
				// continue
			}

			// Identify all the package imports.
			if loc := reImp.FindIndex(line); loc != nil {
				if (commentLoc != nil && commentLoc[0] < loc[0]) ||
					(multiCommentOpenLoc != nil && multiCommentOpenLoc[0] < loc[0]) ||
					(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] > loc[0]) {
					// then it's commented out
					continue
				}

				if match := reImpName.FindSubmatchIndex(line); match != nil {
					pkgName := string(line[match[len(match)-2]:match[len(match)-1]])
					// Checking if `pkgName` already exists and if it's not a standard library package.
					if _, err := cxpartialparsing.Program.GetPackage(preliminaryImportIdent(cxpartialparsing.Program, pkgName)); err != nil && !cxpackages.IsBuiltinImport(pkgName) && !cxpartialparsing.Program.CXProgramImportFailed(preliminaryImportIdent(cxpartialparsing.Program, pkgName)) {
						dir := filepath.Join(globals.SRCPATH, pkgName)
						_, sourceCode, fileNames, err := util.ParseArgsForCX([]string{dir}, false)
						if err != nil {
							// The column is the opening quote of the path.
							preliminaryImportFailed(cxpartialparsing.Program, pkgName, srcNames[i], lineno, match[len(match)-2], preliminaryImportReason(dir, err))
							continue
						}
						preliminaryImport(cxpartialparsing.Program, pkgName, sourceCode, fileNames)
					}
				}
			}

			// we search for packages at the same time, so we can know to what package to add the global
			if loc := rePkg.FindIndex(line); loc != nil {
				if (commentLoc != nil && commentLoc[0] < loc[0]) ||
					(multiCommentOpenLoc != nil && multiCommentOpenLoc[0] < loc[0]) ||
					(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] > loc[0]) {
					// then it's commented out
					continue
				}

				if match := rePkgName.FindStringSubmatch(string(line)); match != nil {
					if pkg, err := cxpartialparsing.Program.GetPackage(cxpartialparsing.Program.CXProgramNameIntern(match[len(match)-1])); err != nil {
						// then it hasn't been added
						// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
						// Now a constructor call, site S1624.
						prePkg = ast.MakePackage(cxpartialparsing.Program.CXProgramNameIntern(match[len(match)-1]))
						// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
						// Now an arena insert, site S1625.
						pkgIdx := cxpartialparsing.Program.AddPackage(prePkg)
						// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
						// Now an arena getter, site S1626.
						prePkg, err = cxpartialparsing.Program.GetPackageFromArray(pkgIdx)
						if err != nil {
							panic(err)
						}
					} else {
						prePkg = pkg
					}
				}
			}

			if locs := reBodyOpen.FindAllIndex(line, -1); locs != nil {
				for _, loc := range locs {
					if !(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] > loc[0]) {
						// then it's outside of a */, e.g. `*/ }`
						if (commentLoc == nil && multiCommentOpenLoc == nil && multiCommentCloseLoc == nil) ||
							(commentLoc != nil && commentLoc[0] > loc[0]) ||
							(multiCommentOpenLoc != nil && multiCommentOpenLoc[0] > loc[0]) ||
							(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] < loc[0]) {
							// then we have an uncommented opening bracket
							inBlock++
						}
					}
				}
			}
			if locs := reBodyClose.FindAllIndex(line, -1); locs != nil {
				for _, loc := range locs {
					if !(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] > loc[0]) {
						if (commentLoc == nil && multiCommentOpenLoc == nil && multiCommentCloseLoc == nil) ||
							(commentLoc != nil && commentLoc[0] > loc[0]) ||
							(multiCommentOpenLoc != nil && multiCommentOpenLoc[0] > loc[0]) ||
							(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] < loc[0]) {
							// then we have an uncommented closing bracket
							inBlock--
						}
					}
				}
			}

			// we could have this situation: {var local i32}
			// but we don't care about this, as the later passes will throw an error as it's invalid syntax

			if loc := rePkg.FindIndex(line); loc != nil {
				if (commentLoc != nil && commentLoc[0] < loc[0]) ||
					(multiCommentOpenLoc != nil && multiCommentOpenLoc[0] < loc[0]) ||
					(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] > loc[0]) {
					// then it's commented out
					continue
				}

				if match := rePkgName.FindStringSubmatch(string(line)); match != nil {
					if pkg, err := cxpartialparsing.Program.GetPackage(cxpartialparsing.Program.CXProgramNameIntern(match[len(match)-1])); err != nil {
						// it should be already present
						panic(err)
					} else {
						prePkg = pkg
					}
				}
			}

			// finally, if we read a "var" and we're in global scope, we add the global without any type
			// the type will be determined later on
			if loc := reGlbl.FindIndex(line); loc != nil {
				if (commentLoc != nil && commentLoc[0] < loc[0]) ||
					(multiCommentOpenLoc != nil && multiCommentOpenLoc[0] < loc[0]) ||
					(multiCommentCloseLoc != nil && multiCommentCloseLoc[0] > loc[0]) || inBlock != 0 {
					// then it's commented out or inside a block
					continue
				}
				if match := reGlblName.FindStringSubmatch(string(line)); match != nil {
					if _, err := prePkg.GetGlobal(actions.AST, actions.AST.CXProgramNameIntern(match[len(match)-1])); err != nil {
						// then it hasn't been added
						_, arg := ast.MakeArgument(actions.AST, actions.AST.CXProgramNameIntern(match[len(match)-1]), ast.SourceFileGidNil, 0)
						arg.Offset = types.InvalidPointer
						arg.Package = ast.CXPackageIndex(prePkg.Index)
						argIdx := ast.CXArgumentIndex(arg.Index)

						prePkg.AddGlobal(actions.AST, argIdx)
					}
				}
			}
		}
		profiling.StopProfile(srcNames[i])
	}
	profiling.StopProfile("2. globals")

	profiling.StartProfile("3. cxpartialparsing")

	for i, source := range srcStrs {
		actions.AST.CxIrState.CxIrStateManagerGrowBoundary()
		profiling.StartProfile(srcNames[i])
		source = source + "\n"
		// Each file selects its package with its package clause.
		actions.AST.CurrentPackage = -1
		cxpartialparsing.CurrentFile = ast.SourceFileGidNil
		if len(srcNames) > 0 {
			cxpartialparsing.CurrentFile = actions.AST.CXProgramSourceFileIntern(srcNames[i])
		}
		/*
			passone
		*/
		parseErrors += Passone(source)
		profiling.StopProfile(srcNames[i])
	}

	profiling.StopProfile("3. cxpartialparsing")
	return parseErrors, packages
}

func AddInitFunction(prgrm *ast.CXProgram) error {
	prgrm.CxIrState.CxIrStateManagerGrowBoundary()
	mainPkg, err := prgrm.GetPackage(prgrm.CxIrState.CxIrStateNameMain)
	if err != nil {
		return err
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
	// Now a constructor call, site S1629.
	initFn := ast.MakeFunction(prgrm, prgrm.CxIrState.CxIrStateNameSysInit, actions.CurrentFile, actions.LineNo)
	_, fnIdx := mainPkg.AddFunction(prgrm, initFn)
	globalsMethodResultsDeclare(prgrm, prgrm.GetFunctionFromArray(fnIdx))

	globalsInitializersOrder(prgrm)
	//Init Expressions
	actions.FunctionDeclaration(prgrm, fnIdx, nil, nil, prgrm.SysInitExprs)

	if _, err := mainPkg.SelectFunction(prgrm, prgrm.CxIrState.CxIrStateNameMain); err != nil {
		return err
	}
	return nil
}

// preliminaryImportFailed records that the import of path at line and column
// of the file name cannot be loaded, and adds an empty package in its place, so
// the passes import it and its members report no error of their own.
func preliminaryImportFailed(prgrm *ast.CXProgram, path string, name string, line int, column int, reason string) {
	prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeImportFailed, prgrm.CXProgramSourceFileIntern(name), line, column, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(path), prgrm.CXProgramCompileErrorText(reason)})
	ident := preliminaryImportIdent(prgrm, path)
	prgrm.CXProgramImportFailedAdd(ident)
	if _, err := prgrm.GetPackage(ident); err != nil {
		prgrm.AddPackage(ast.MakePackage(ident))
	}
}

// preliminaryImportIdent is the name of the package of the import path.
func preliminaryImportIdent(prgrm *ast.CXProgram, path string) stringpool.StringGid {
	return prgrm.CXProgramNameIntern(path[strings.LastIndexByte(path, '/')+1:])
}

// preliminaryImport compiles the files of the package of the import path.
// Their errors print with the errors of the program, and a package with
// errors is a failed import, whose members report no error of their own.
func preliminaryImport(prgrm *ast.CXProgram, path string, sourceCode []*os.File, fileNames []string) {
	sources := make([]string, len(sourceCode))
	for index, file := range sourceCode {
		text, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			panic(err)
		}
		sources[index] = string(text)
	}
	for index := range fileNames {
		fileNames[index] = preliminaryImportPath(fileNames[index])
	}
	records := len(prgrm.CxIrState.CompileErrorList.CompileErrorListRecords)
	// The error return also counts the errors recorded before the import.
	parseSourceStrings(sources, fileNames)
	if len(prgrm.CxIrState.CompileErrorList.CompileErrorListRecords) == records {
		return
	}
	ident := preliminaryImportIdent(prgrm, path)
	prgrm.CXProgramImportFailedAdd(ident)
	if pkg, err := prgrm.GetPackage(ident); err == nil {
		clear(pkg.Functions)
		clear(pkg.Structs)
		clear(pkg.Methods)
		pkg.Globals.Fields = nil
	}
}

// preliminaryImportPath is path relative to the working directory, or to
// the parent of SRCPATH as $CXPATH/..., so the errors do not depend on where
// the program is.
func preliminaryImportPath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, path); err == nil && filepath.IsLocal(rel) {
			return filepath.ToSlash(rel)
		}
	}
	if root, err := filepath.Abs(filepath.Dir(globals.SRCPATH)); err == nil {
		if rel, err := filepath.Rel(root, path); err == nil && filepath.IsLocal(rel) {
			return "$CXPATH/" + filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

// preliminaryImportReason is the text of err, which loading dir returned,
// with dir as preliminaryImportPath prints it.
func preliminaryImportReason(dir string, err error) string {
	shown := preliminaryImportPath(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return shown + ": source file or library not found"
	}
	return strings.ReplaceAll(err.Error(), dir, shown)
}
