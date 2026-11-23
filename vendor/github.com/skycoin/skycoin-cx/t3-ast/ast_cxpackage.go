package ast

import (
	"errors"
	"fmt"

	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

type CXPackageIndex int

// CXPackageMethodKey names a method by the name of its receiver struct and
// its own name.
type CXPackageMethodKey struct {
	CXPackageMethodKeyStruct stringpool.StringGid
	CXPackageMethodKeyMethod stringpool.StringGid
}

// Only Used by Affordances in op_aff.go
func (pkg *CXPackage) GetFunction(prgrm *CXProgram, fnName stringpool.StringGid) (*CXFunction, error) {
	if fnIdx, ok := pkg.Functions[fnName]; ok {
		// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
		// Now an arena getter, site S0158.
		fn := prgrm.GetFunctionFromArray(fnIdx)

		return fn, nil
	}

	// now checking in imported packages
	for _, impIdx := range pkg.Imports {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0159.
		imp, err := prgrm.GetPackageFromArray(impIdx)
		if err != nil {
			panic(err)
		}

		if fnIdx, ok := imp.Functions[fnName]; ok {
			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now an arena getter, site S0160.
			fn := prgrm.GetFunctionFromArray(fnIdx)

			return fn, nil
		}
	}

	return nil, fmt.Errorf("function '%s' not found in package '%s' or its imports", prgrm.CXProgramNameText(fnName), prgrm.CXProgramNameText(pkg.Name))
}

// GetImport ...
func (pkg *CXPackage) GetImport(prgrm *CXProgram, impName stringpool.StringGid) (*CXPackage, error) {
	if _, ok := pkg.Imports[impName]; !ok {
		return nil, fmt.Errorf("package '%s' not imported", prgrm.CXProgramNameText(impName))
	}

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0161.
	imp, err := prgrm.GetPackageFromArray(pkg.Imports[impName])
	if err != nil {
		panic(err)
	}
	return imp, nil
}

// GetMethod returns the method methodName of the struct receiverName.
func (pkg *CXPackage) GetMethod(prgrm *CXProgram, receiverName stringpool.StringGid, methodName stringpool.StringGid) (CXFunctionIndex, error) {
	if fnIdx, ok := pkg.Methods[CXPackageMethodKey{CXPackageMethodKeyStruct: receiverName, CXPackageMethodKeyMethod: methodName}]; ok {
		return fnIdx, nil
	}

	// Trying to find it in `Natives`.
	// Most likely a method from a core package. OpCodes stays keyed by text
	// until CXNativeFunction names are Gids, see #413.
	if opCode, found := OpCodes[prgrm.CXProgramNameText(pkg.Name)+"."+prgrm.CXProgramNameText(receiverName)+"."+prgrm.CXProgramNameText(methodName)]; found {
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now an arena insert, site S0167.
		opFnIdx := prgrm.AddNativeFunctionInArray(Natives[opCode])
		return opFnIdx, nil
	}

	return -1, fmt.Errorf("method '%s.%s' not found in package '%s'", prgrm.CXProgramNameText(receiverName), prgrm.CXProgramNameText(methodName), prgrm.CXProgramNameText(pkg.Name))
}

// GetStruct ...
func (pkg *CXPackage) GetStruct(prgrm *CXProgram, strctName stringpool.StringGid) (*CXStruct, error) {
	if strctIdx, ok := pkg.Structs[strctName]; ok {
		// OBJMODEL: state.CXStructMng.CXStructGet(gid), phase 5 #337.
		// Now a pointer into an arena, site S0168.
		return &prgrm.CXStructs[strctIdx], nil
	}

	// looking in imports
	for _, impIdx := range pkg.Imports {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0169.
		imp, err := prgrm.GetPackageFromArray(impIdx)
		if err != nil {
			panic(err)
		}
		if strctIdx, ok := imp.Structs[strctName]; ok {
			// OBJMODEL: state.CXStructMng.CXStructGet(gid), phase 5 #337.
			// Now a pointer into an arena, site S0170.
			return &prgrm.CXStructs[strctIdx], nil
		}
	}

	return nil, fmt.Errorf("struct '%s' not found in package '%s'", prgrm.CXProgramNameText(strctName), prgrm.CXProgramNameText(pkg.Name))

}

// GetGlobal ...
func (pkg *CXPackage) GetGlobal(prgrm *CXProgram, defName stringpool.StringGid) (*CXTypeSignature, error) {
	for _, fieldIdx := range pkg.Globals.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0171.
		field := prgrm.GetCXTypeSignatureFromArray(fieldIdx)
		if field.Name == defName {
			return field, nil
		}
	}

	return nil, fmt.Errorf("global '%s' not found in package '%s'", prgrm.CXProgramNameText(defName), prgrm.CXProgramNameText(pkg.Name))
}

// GetCurrentFunction ...
func (pkg *CXPackage) GetCurrentFunction(prgrm *CXProgram) (*CXFunction, error) {
	if pkg.CurrentFunction == -1 {
		return nil, errors.New("current function is nil")
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0172.
	return prgrm.GetFunctionFromArray(pkg.CurrentFunction), nil
}

// ----------------------------------------------------------------
//                             `CXPackage` Selectors

// SelectFunction ...
func (pkg *CXPackage) SelectFunction(prgrm *CXProgram, name stringpool.StringGid) (*CXFunction, error) {
	if _, ok := pkg.Functions[name]; !ok {
		return nil, fmt.Errorf("function '%s' does not exist", prgrm.CXProgramNameText(name))
	}

	idx := pkg.Functions[name]
	pkg.CurrentFunction = idx

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0173.
	return prgrm.GetFunctionFromArray(idx), nil
}

// MakePackage creates a new empty CXPackage.
//
// It can be filled in later with imports, structs, globals and functions.
// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
// Now a constructor, site S0174.
func MakePackage(name stringpool.StringGid) *CXPackage {
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now a heap literal, site S0175.
	return &CXPackage{
		Name: name,
		// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
		// Now a heap literal, site S0176.
		Globals:         &CXStruct{},
		Imports:         make(map[stringpool.StringGid]CXPackageIndex, 0),
		Structs:         make(map[stringpool.StringGid]CXStructIndex, 0),
		Functions:       make(map[stringpool.StringGid]CXFunctionIndex, 0),
		Methods:         make(map[CXPackageMethodKey]CXFunctionIndex, 0),
		CurrentFunction: -1,
	}
}

// ----------------------------------------------------------------
//                             `CXPackage` Getters

// ----------------------------------------------------------------
//                     `CXPackage` Member handling

// AddImport ...
func (pkg *CXPackage) AddImport(prgrm *CXProgram, imp *CXPackage) *CXPackage {
	if _, ok := pkg.Imports[imp.Name]; !ok {
		// impIdx := prgrm.AddPackageInArray(imp)
		pkg.Imports[imp.Name] = CXPackageIndex(imp.Index)
	}

	return pkg
}

// RemoveImport ...
func (pkg *CXPackage) RemoveImport(impName stringpool.StringGid) {
	if _, ok := pkg.Imports[impName]; ok {
		// OBJMODEL: state.CXPackageMng.CXPackageRelease(gid), phase 4 #361.
		// Now a dropped object, site S0177.
		delete(pkg.Imports, impName)
	}
}

// AddFunction adds fn to pkg. A second function of the same name is an
// error and stays out of pkg.Functions, so the first keeps the name as in Go.
func (pkg *CXPackage) AddFunction(prgrm *CXProgram, fn *CXFunction) (*CXPackage, CXFunctionIndex) {
	return pkg, pkg.addFunction(prgrm, fn, CompileErrorCodeRedeclaredBlock)
}

func (pkg *CXPackage) addFunction(prgrm *CXProgram, fn *CXFunction, code CompileErrorCode) CXFunctionIndex {
	fn.Package = CXPackageIndex(pkg.Index)

	// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
	// Now an arena insert, site S0178.
	fnIdx := prgrm.AddFunctionInArray(fn)
	if firstIdx, ok := pkg.Functions[fn.Name]; ok {
		first := prgrm.GetFunctionFromArray(firstIdx)
		column := fn.FileColumn
		if column == 0 {
			column = prgrm.CXProgramCompileErrorColumn(fn.FileName, fn.FileLine)
		}
		prgrm.CXProgramCompileErrorAddPosition(code, fn.FileName, fn.FileLine, column, CompileErrorOperands{int64(fn.Name), int64(first.FileName), int64(first.FileLine), int64(first.FileColumn)})
	} else {
		pkg.Functions[fn.Name] = fnIdx
	}
	pkg.CurrentFunction = fnIdx

	return fnIdx
}

// AddMethod adds fn, the method methodName of the struct receiverName. The
// name of fn is "receiverName.methodName", see CXProgramMethodNameIntern.
func (pkg *CXPackage) AddMethod(prgrm *CXProgram, fn *CXFunction, receiverName stringpool.StringGid, methodName stringpool.StringGid) (*CXPackage, CXFunctionIndex) {
	key := CXPackageMethodKey{CXPackageMethodKeyStruct: receiverName, CXPackageMethodKeyMethod: methodName}
	_, declared := pkg.Methods[key]
	fnIdx := pkg.addFunction(prgrm, fn, CompileErrorCodeMethodRedeclared)
	if !declared {
		pkg.Methods[key] = fnIdx
	}
	return pkg, fnIdx
}

// FunctionDeclarationAt returns the function declared at file, line and
// column with the name and package of fnIdx. It is fnIdx unless the name was
// declared twice, or for a line of 0.
func (prgrm *CXProgram) FunctionDeclarationAt(fnIdx CXFunctionIndex, file SourceFileGid, line int, column int) CXFunctionIndex {
	fn := prgrm.GetFunctionFromArray(fnIdx)
	if line == 0 || fn.FileName == file && fn.FileLine == line && fn.FileColumn == column {
		return fnIdx
	}
	for index := range prgrm.CXFunctions {
		other := &prgrm.CXFunctions[index]
		if other.Name == fn.Name && other.Package == fn.Package && other.FileName == file && other.FileLine == line && other.FileColumn == column {
			return CXFunctionIndex(index)
		}
	}
	return fnIdx
}

// RemoveFunction ...
func (pkg *CXPackage) RemoveFunction(fnName stringpool.StringGid) {
	fnIdx, ok := pkg.Functions[fnName]
	if !ok {
		return
	}
	for key, methodIdx := range pkg.Methods {
		if methodIdx == fnIdx {
			delete(pkg.Methods, key)
		}
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionRelease(gid), phase 4 #351.
	// Now a dropped object, site S0179.
	delete(pkg.Functions, fnName)
}

// AddStruct ...
func (pkg *CXPackage) AddStruct(prgrm *CXProgram, strct *CXStruct) *CXPackage {
	if _, ok := pkg.Structs[strct.Name]; ok {
		return pkg
	}
	strct.Package = CXPackageIndex(pkg.Index)
	// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
	// Now an arena insert, site S0180.
	strctIdx := prgrm.AddStructInArray(strct)
	pkg.Structs[strct.Name] = strctIdx

	return pkg
}

// RemoveStruct ...
func (pkg *CXPackage) RemoveStruct(strctName stringpool.StringGid) {
	if _, ok := pkg.Structs[strctName]; !ok {
		return
	}
	// OBJMODEL: state.CXStructMng.CXStructRelease(gid), phase 4 #336.
	// Now a dropped object, site S0181.
	delete(pkg.Structs, strctName)
}

// AddGlobal ...
func (pkg *CXPackage) AddGlobal(prgrm *CXProgram, defIdx CXArgumentIndex) *CXPackage {
	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S0182.
	defArg := prgrm.GetCXArgFromArray(defIdx)
	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S1631.
	prgrm.GetCXArg(CXArgumentIndex(defIdx)).Package = CXPackageIndex(pkg.Index)

	for _, fieldIdx := range pkg.Globals.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0183.
		field := prgrm.GetCXTypeSignatureFromArray(fieldIdx)
		if field.Name == defArg.Name && field.Type == TYPE_CXARGUMENT_DEPRECATE {
			return pkg
		}
	}

	pkg.Globals.AddField_Globals_CXAtomicOps(prgrm, defIdx)

	return pkg
}

func (pkg *CXPackage) AddGlobal_TypeSignature(prgrm *CXProgram, typeSignatureIdx CXTypeSignatureIndex) *CXPackage {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S0184.
	typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
	typeSignature.Package = CXPackageIndex(pkg.Index)
	for _, fieldIdx := range pkg.Globals.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0185.
		field := prgrm.GetCXTypeSignatureFromArray(fieldIdx)
		if field.Name == typeSignature.Name {
			return pkg
		}
	}

	pkg.Globals.AddField_TypeSignature(prgrm, typeSignatureIdx)

	return pkg
}

// RemoveGlobal ...
// func (pkg *CXPackage) RemoveGlobal(prgrm *CXProgram, defName string) {
// 	lenGlobals := len(pkg.Globals)
// 	for i, defIdx := range pkg.Globals {
// 		def := prgrm.GetCXArg(defIdx)
// 		if def.Name == defName {
// 			if i == lenGlobals-1 {
// 				pkg.Globals = pkg.Globals[:len(pkg.Globals)-1]
// 			} else {
// 				pkg.Globals = append(pkg.Globals[:i], pkg.Globals[i+1:]...)
// 			}
// 			break
// 		}
// 	}
// }
