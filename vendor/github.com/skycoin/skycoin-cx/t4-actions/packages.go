package actions

import (
	"fmt"
	"strings"

	cxpackages "github.com/skycoin/skycoin-cx/t0-packages" // thin: only CheckImport, GuiPackage, IsDefaultPackage; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// DeclarePackage() switches the current package in the program.
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	pkgName - name of the package to declare.
func DeclarePackage(prgrm *ast.CXProgram, pkgName string) {
	pkgNameGid := prgrm.CXProgramNameIntern(pkgName)
	// Add a new package to the program if it's not previously defined.
	if _, err := prgrm.GetPackage(pkgNameGid); err != nil {
		// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
		// Now a constructor call, site S1088.
		pkg := ast.MakePackage(pkgNameGid)
		// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
		// Now an arena insert, site S1089.
		prgrm.AddPackage(pkg)
	}

	_, err := prgrm.SelectPackage(pkgNameGid)
	if err != nil {
		panic(err)
	}
}

// DeclareImport()
//
// Input arguments description:
//
//	prgrm - a CXProgram that contains all the data and arrays of the program.
//	impName - name of the import to declare.
//	currentFile - the current cx source code file.
//	lineNo - the current line number from the source code.
func DeclareImport(prgrm *ast.CXProgram, impName string, currentFile ast.SourceFileGid, lineNo int) {
	// Make sure we are inside a package
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		// FIXME: Should give a relevant error message
		panic(err)
	}

	if err := cxpackages.CheckImport(impName); err != nil {
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeMessage, currentFile, lineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(err.Error())})
		return
	}
	if name, found := strings.CutPrefix(impName, cxpackages.SysPrefix); found && cxpackages.GuiPackage(name) && !cxpackages.GuiBuild {
		prgrm.CXProgramCompileErrorAdd(ast.CompileErrorCodeImportGuiBuildMissing, currentFile, lineNo, ast.CompileErrorOperands{prgrm.CXProgramCompileErrorText(impName)})
		return
	}

	// Checking if it's a package in the CX workspace by trying to find a
	// slash (/) in the impName.
	// We start backwards and we stop if we find a slash.
	hasSlash := false
	c := len(impName) - 1
	for ; c >= 0; c-- {
		if impName[c] == '/' {
			hasSlash = true
			break
		}
	}
	ident := ""
	// If the `impName` has a slash, then we need to strip
	// everything behind the slash and the slash itself.
	if hasSlash {
		ident = impName[c+1:]
	} else {
		ident = impName
	}

	identGid := prgrm.CXProgramNameIntern(ident)

	// If the package is already imported, then there is nothing more to be done.
	if _, err := pkg.GetImport(prgrm, identGid); err == nil {
		return
	}

	// If the package is already defined in the program, just add it to
	// the importing package.
	if imp, err := prgrm.GetPackage(identGid); err == nil {
		pkg.AddImport(prgrm, imp)
		return
	}

	// All packages are read during the first pass of the compilation.  So
	// if we get here during the 2nd pass, it's either a core package or
	// something is panic-level wrong.
	if cxpackages.IsDefaultPackage(ident) {
		// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
		// Now a constructor call, site S1090.
		imp := ast.MakePackage(identGid)
		// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
		// Now an arena insert, site S1091.
		impIdx := prgrm.AddPackage(imp)
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S1092.
		newImp, err := prgrm.GetPackageFromArray(impIdx)
		if err != nil {
			panic(err)
		}
		pkg.AddImport(prgrm, newImp)

		prgrm.CurrentPackage = ast.CXPackageIndex(pkg.Index)

		if ident == "aff" {
			AffordanceStructs(prgrm, newImp, currentFile, lineNo)
		}
	} else {
		// This should never happen.
		panic(fmt.Sprintf("error: %s:%d: unkown error when trying to read package '%s'", prgrm.CXProgramSourceFileText(currentFile), lineNo, ident))
	}
}
