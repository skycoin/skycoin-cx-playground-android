package actions

import (
	"sort"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// DeclareStruct takes a name of a struct and a slice of fields representing
// the members and adds the struct to the package.
//
// Input arguments description:
//
//		prgrm - a CXProgram that contains all the data and arrays of the program.
//		structName - name of the struct to declare.
//	 structFields - fields of the struct to be added.
//	 file, line, column - the position of the name. Both passes declare each
//				   struct, and a struct of the package declared at another
//				   position is a redeclaration.
func DeclareStruct(prgrm *ast.CXProgram, structName string, structFields []*ast.CXArgument, file ast.SourceFileGid, line int, column int) {
	// Make sure we are inside a package.
	pkg, err := prgrm.GetCurrentPackage()
	if err != nil {
		// FIXME: Should give a relevant error message
		panic(err)
	}

	// Make sure a struct with the same name is not yet defined.
	strct, err := prgrm.GetStruct(prgrm.CXProgramNameIntern(structName), pkg.Name)
	if err != nil {
		// FIXME: Should give a relevant error message
		panic(err)
	}

	if strct.FileLine != 0 && (strct.FileName != file || strct.FileLine != line || strct.FileColumn != column) {
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeRedeclaredBlock, file, line, column, ast.CompileErrorOperands{int64(strct.Name), int64(strct.FileName), int64(strct.FileLine), int64(strct.FileColumn)})
		for _, field := range structFields {
			prgrm.ReleaseCXArgBuilder(field)
		}
		return
	}
	strct.FileName, strct.FileLine, strct.FileColumn = file, line, column

	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureRelease(gid), phase 4 #331.
	// Now a dropped object, site S1208.
	strct.Fields = nil
	var structSize types.Pointer
	for _, field := range structFields {
		fieldSize, fits := declarationArgSize(field)
		if !fits || !declarationSizeSumFits(structSize, fieldSize) {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeDeclarationTooLarge, field.ArgDetails, ast.CompileErrorOperands{int64(field.Name)})
			prgrm.ReleaseCXArgBuilder(field)
		} else if _, err := strct.GetField(prgrm, field.Name); err == nil {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeStructFieldMultiplyDefined, field.ArgDetails, ast.CompileErrorOperands{int64(field.Name)})
			prgrm.ReleaseCXArgBuilder(field)
		} else if inner := declareStructArgValueStruct(field); inner != nil && declareStructContains(prgrm, inner, strct) {
			prgrm.CXProgramCompileErrorAddDebug(ast.CompileErrorCodeStructRecursive, field.ArgDetails, ast.CompileErrorOperands{int64(strct.Name)})
			prgrm.ReleaseCXArgBuilder(field)
		} else {
			structSize += fieldSize
			strct.AddField(prgrm, field)
		}
	}
}

// declareStructArgValueStruct returns the struct arg holds as a plain value,
// or nil for no struct or one behind a pointer, slice or array (issue-233).
func declareStructArgValueStruct(arg *ast.CXArgument) *ast.CXStruct {
	for _, decl := range arg.DeclarationSpecifiers {
		if decl == constants.DECL_POINTER || decl == constants.DECL_SLICE || decl == constants.DECL_ARRAY {
			return nil
		}
	}
	return arg.StructType
}

// declareStructContains reports whether a value of strct holds target by
// value. Declared structs never hold themselves, so the walk ends.
func declareStructContains(prgrm *ast.CXProgram, strct *ast.CXStruct, target *ast.CXStruct) bool {
	if strct == target {
		return true
	}
	for _, fieldIdx := range strct.Fields {
		field := prgrm.GetCXTypeSignatureFromArray(fieldIdx)
		var inner *ast.CXStruct
		switch field.Type {
		case ast.TYPE_STRUCT:
			inner = prgrm.GetCXTypeSignatureStructFromArray(field.Meta).StructType
		case ast.TYPE_CXARGUMENT_DEPRECATE:
			inner = declareStructArgValueStruct(prgrm.GetCXArgFromArray(ast.CXArgumentIndex(field.Meta)))
		}
		if inner != nil && declareStructContains(prgrm, inner, target) {
			return true
		}
	}
	return false
}

// packageDeclaration is the name and position of a type, function or global.
type packageDeclaration struct {
	name   stringpool.StringGid
	file   ast.SourceFileGid
	line   int
	column int
}

// PackageRedeclarationCheck records each name that pkg declares as two of a
// type, a function and a global, at the later declaration. A second
// declaration of the same kind is recorded where it is declared.
func PackageRedeclarationCheck(prgrm *ast.CXProgram, pkg *ast.CXPackage) {
	var declarations []packageDeclaration
	for _, strctIdx := range pkg.Structs {
		strct := prgrm.GetStructFromArray(strctIdx)
		declarations = append(declarations, packageDeclaration{strct.Name, strct.FileName, strct.FileLine, strct.FileColumn})
	}
	methods := make(map[ast.CXFunctionIndex]bool, len(pkg.Methods))
	for _, fnIdx := range pkg.Methods {
		methods[fnIdx] = true
	}
	for _, fnIdx := range pkg.Functions {
		if !methods[fnIdx] {
			fn := prgrm.GetFunctionFromArray(fnIdx)
			declarations = append(declarations, packageDeclaration{fn.Name, fn.FileName, fn.FileLine, fn.FileColumn})
		}
	}
	for _, glblIdx := range pkg.Globals.Fields {
		glbl := prgrm.GetCXTypeSignatureFromArray(glblIdx)
		file, line, column := ast.CXArgumentDebugPosition(prgrm, glbl.ArgDetails)
		declarations = append(declarations, packageDeclaration{glbl.Name, file, line, column})
	}
	sort.Slice(declarations, func(left, right int) bool {
		a, b := &declarations[left], &declarations[right]
		if a.file != b.file {
			return a.file < b.file
		}
		if a.line != b.line {
			return a.line < b.line
		}
		return a.column < b.column
	})
	first := make(map[stringpool.StringGid]int, len(declarations))
	for index, declaration := range declarations {
		if declaration.line <= 0 {
			continue
		}
		other, ok := first[declaration.name]
		if !ok {
			first[declaration.name] = index
			continue
		}
		prev := &declarations[other]
		prgrm.CXProgramCompileErrorAddPosition(ast.CompileErrorCodeRedeclaredBlock, declaration.file, declaration.line, declaration.column, ast.CompileErrorOperands{int64(declaration.name), int64(prev.file), int64(prev.line), int64(prev.column)})
	}
}
