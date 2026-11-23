package ast

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/skycoin/skycoin-cx/t0-types"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

type StackSegmentStruct struct {
	Size     types.Pointer // This field stores the size of a CX program's stack
	StartsAt types.Pointer // Offset at which the stack segment starts in a CX program's memory
	Pointer  types.Pointer // At what byte the current stack frame is
}

type DataSegmentStruct struct {
	Size     types.Pointer // This field stores the size of a CX program's data segment size
	StartsAt types.Pointer // Offset at which the data segment starts in a CX program's memory
}

// HeapVisit is a pointer or slice the mark followed, and its GCType.
type HeapVisit struct {
	HeapVisitAddress types.Pointer
	HeapVisitType    *GCType
}

type HeapSegmentStruct struct {
	Size     types.Pointer // This field stores the size of a CX program's heap
	StartsAt types.Pointer // Offset at which the heap starts in a CX program's memory (normally the stack size)
	Pointer  types.Pointer // At what offset a CX program can insert a new object to the heap
	// ObjectStarts lists the address of each object, for the collection running.
	ObjectStarts []types.Pointer
	// Slots lists the address of each reference the mark read.
	Slots []types.Pointer
	// Visited holds the pointers and slices the mark followed.
	Visited map[HeapVisit]bool
	// Work holds the pointers and slices the mark has yet to follow.
	Work []HeapVisit
	// Pinned is an address inside an object that Go code holds across an
	// allocation, or 0. The collection keeps its object and moves it.
	Pinned types.Pointer
	// Types holds the GCTypes of the roots, built by the first collection.
	Types *GCTypeCache
}

// ----------------------------------------------------------------
//                         `CXProgram` Package handling

// AddPackage ...
func (cxprogram *CXProgram) AddPackage(mod *CXPackage) CXPackageIndex {
	if idx, ok := cxprogram.Packages[mod.Name]; ok {
		cxprogram.CurrentPackage = idx
		return idx
	}

	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena insert, site S0188.
	index := cxprogram.AddPackageInArray(mod)

	cxprogram.Packages[mod.Name] = CXPackageIndex(index)
	cxprogram.CurrentPackage = CXPackageIndex(index)

	return index
}

func (cxprogram *CXProgram) AddPackageInArray(pkg *CXPackage) CXPackageIndex {
	// The index of pkg after it will be added in the array
	pkg.Index = len(cxprogram.CXPackages)
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena append, site S0189.
	cxprogram.CXPackages = append(cxprogram.CXPackages, *pkg)

	return CXPackageIndex(pkg.Index)
}

func (cxprogram *CXProgram) GetPackageFromArray(index CXPackageIndex) (*CXPackage, error) {
	if int(index) > (len(cxprogram.CXPackages) - 1) {
		return nil, fmt.Errorf("error: CXPackages[%d]: index out of bounds", index)
	}

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now a pointer into an arena, site S0190.
	return &cxprogram.CXPackages[index], nil
}

// RemovePackage ...
// func (cxprogram *CXProgram) RemovePackage(modName string) {
// 	// If doesnt exist, return
// 	// if cxprogram.Packages[modName] == nil {
// 	// 	return
// 	// }

// 	// Check if it is the current pkg so when it
// 	// is deleted, it will be replaced with new pkg
// 	isCurrentPkg := cxprogram.Packages[modName] == cxprogram.CurrentPackage

// 	// Delete package
// 	delete(cxprogram.Packages, modName)

// 	// This means that we're removing the package set to be the CurrentPackage.
// 	// If it is removed from the program's map of packages, cxprogram.CurrentPackage
// 	// would be pointing to a package meant to be collected by the GC.
// 	// We fix this by pointing to random package in the program's map of packages.
// 	if isCurrentPkg {
// 		for _, pkg := range cxprogram.Packages {
// 			cxprogram.CurrentPackage = pkg
// 			break
// 		}
// 	}

// }

// ----------------------------------------------------------------
//
//	`CXProgram` Function handling
func (cxprogram *CXProgram) AddFunctionInArray(fn *CXFunction) CXFunctionIndex {
	// The index of fn after it will be added in the array
	fn.Index = len(cxprogram.CXFunctions)

	// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
	// Now an arena append, site S0191.
	cxprogram.CXFunctions = append(cxprogram.CXFunctions, *fn)

	return CXFunctionIndex(fn.Index)
}

// nativeArgCopy returns a copy of a native argument for one program.
// Natives are shared by every program, so their Index must stay unset.
// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
// Now a constructor, site S0192.
func nativeArgCopy(arg *CXArgument) *CXArgument {
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now an object copy, site S0193.
	cp := *arg
	cp.Index = -1
	return &cp
}

func (cxprogram *CXProgram) AddNativeFunctionInArray(fn *CXNativeFunction) CXFunctionIndex {
	// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
	// Now a heap literal, site S0194.
	fnNative := &CXFunction{
		Index:        len(cxprogram.CXFunctions),
		AtomicOPCode: fn.AtomicOPCode,
		// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
		// Now a heap literal, site S0195.
		Inputs: &CXStruct{},
		// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
		// Now a heap literal, site S0196.
		Outputs:        &CXStruct{},
		LocalVariables: []stringpool.StringGid{},
	}

	// Add inputs to cx arg array
	for _, argIn := range fn.Inputs {
		err := fnNative.AddLocalVariableName(argIn.Name)
		if err != nil {
			// TODO: improve error handling
			panic("error adding local variable name")
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now a constructor call, site S0197.
		newField := GetCXTypeSignatureRepresentationOfCXArg(cxprogram, nativeArgCopy(argIn))

		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureAlloc(), phase 4 #331.
		// Now an arena insert, site S0198.
		newFieldIdx := cxprogram.AddCXTypeSignatureInArray(newField)
		fnNative.Inputs.Fields = append(fnNative.Inputs.Fields, newFieldIdx)
	}

	// Add outputs to cx arg array
	for _, argOut := range fn.Outputs {
		err := fnNative.AddLocalVariableName(argOut.Name)
		if err != nil {
			// TODO: improve error handling
			panic("error adding local variable name")
		}

		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now a constructor call, site S0199.
		newField := GetCXTypeSignatureRepresentationOfCXArg(cxprogram, nativeArgCopy(argOut))
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureAlloc(), phase 4 #331.
		// Now an arena insert, site S0200.
		newFieldIdx := cxprogram.AddCXTypeSignatureInArray(newField)
		fnNative.Outputs.Fields = append(fnNative.Outputs.Fields, newFieldIdx)
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
	// Now an arena append, site S0201.
	cxprogram.CXFunctions = append(cxprogram.CXFunctions, *fnNative)

	return CXFunctionIndex(fnNative.Index)
}

func (cxprogram *CXProgram) GetFunctionFromArray(index CXFunctionIndex) *CXFunction {
	if index == -1 {
		return nil
	}

	if int(index) > (len(cxprogram.CXFunctions) - 1) {
		panic(fmt.Errorf("error: CXFunctions[%d]: index out of bounds", index))
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now a pointer into an arena, site S0202.
	return &cxprogram.CXFunctions[index]
}

// ----------------------------------------------------------------
//
//	`CXProgram` Structs handling
func (cxprogram *CXProgram) AddStructInArray(strct *CXStruct) CXStructIndex {
	// The index of fn after it will be added in the array
	strct.Index = len(cxprogram.CXStructs)

	// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
	// Now an arena append, site S0203.
	cxprogram.CXStructs = append(cxprogram.CXStructs, *strct)

	return CXStructIndex(strct.Index)
}

func (cxprogram *CXProgram) GetStructFromArray(index CXStructIndex) *CXStruct {
	if index == -1 {
		return nil
	}

	if int(index) > (len(cxprogram.CXStructs) - 1) {
		panic(fmt.Errorf("error: CXStructs[%d]: index out of bounds", index))
	}

	// OBJMODEL: state.CXStructMng.CXStructGet(gid), phase 5 #337.
	// Now a pointer into an arena, site S0204.
	return &cxprogram.CXStructs[index]
}

// ----------------------------------------------------------------
//            `CXProgram` CXLines, CXArgs, and CXAtomicOps Handling

// GetCXLine returns the line in slot index of the program's CXLineMng. An
// expression keeps the slot index, not the Gid, until phase 5 of CXLine.
func (cxprogram *CXProgram) GetCXLine(index int) (*CXLine, error) {
	manager := &cxprogram.CxIrState.CXLineMng
	if index < 0 || index >= int(manager.CXLineMngNext) {
		return nil, fmt.Errorf("error: CXLines[%d]: index out of bounds", index)
	}

	gid := objmodel.GidCompose(CXLineGidTypeTag, manager.CXLineMngGenerations[index], uint32(index))
	return manager.CXLineGet(CXLineGid(gid)), nil
}

// GetCXArg returns the argument in slot index of the program's
// CXArgumentMng. An argument is named by its slot index, not its Gid, until
// phase 5 of CXArgument.
func (cxprogram *CXProgram) GetCXArg(index CXArgumentIndex) *CXArgument {
	manager := &cxprogram.CxIrState.CXArgumentMng
	if index < 0 || int(index) >= int(manager.CXArgumentMngNext) {
		panic(fmt.Errorf("error: CXArgs[%d]: index out of bounds", index))
	}

	gid := objmodel.GidCompose(CXArgumentGidTypeTag, manager.CXArgumentMngGenerations[index], uint32(index))
	return manager.CXArgumentGet(CXArgumentGid(gid))
}

func (cxprogram *CXProgram) GetCXAtomicOp(index int) (*CXAtomicOperator, error) {
	if index > (len(cxprogram.CXAtomicOps) - 1) {
		return nil, fmt.Errorf("error: CXAtomicOps[%d]: index out of bounds", index)
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now a pointer into an arena, site S0207.
	return &cxprogram.CXAtomicOps[index], nil
}

func (cxprogram *CXProgram) AddCXAtomicOp(CXAtomicOp *CXAtomicOperator) int {
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
	// Now an arena append, site S0209.
	cxprogram.CXAtomicOps = append(cxprogram.CXAtomicOps, *CXAtomicOp)

	return len(cxprogram.CXAtomicOps) - 1
}

// ----------------------------------------------------------------
//
//	`CXProgram` CXTypeSignatures handling

// AddCXTypeSignatureInArray allocates a type signature in the program's
// CXTypeSignatureMng and copies typeSignature into it. typeSignature.Index is
// set to the slot index, which it returns.
func (cxprogram *CXProgram) AddCXTypeSignatureInArray(typeSignature *CXTypeSignature) CXTypeSignatureIndex {
	gid := cxprogram.CxIrState.CXTypeSignatureMng.CXTypeSignatureAlloc()
	typeSignature.Index = CXTypeSignatureIndex(objmodel.GidIndex(int64(gid)))
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureAlloc(), phase 4 #331.
	// Now an object copy, site S1775.
	*cxprogram.CxIrState.CXTypeSignatureMng.CXTypeSignatureGet(gid) = *typeSignature

	return typeSignature.Index
}

// AllocCXTypeSignature allocates a type signature in the program's
// CXTypeSignatureMng, sets its Index to the slot and returns its Gid and the
// pointer to fill it through.
func (cxprogram *CXProgram) AllocCXTypeSignature() (CXTypeSignatureGid, *CXTypeSignature) {
	manager := &cxprogram.CxIrState.CXTypeSignatureMng
	gid := manager.CXTypeSignatureAlloc()
	typeSignature := manager.CXTypeSignatureGet(gid)
	typeSignature.Index = CXTypeSignatureIndex(objmodel.GidIndex(int64(gid)))
	return gid, typeSignature
}

// AllocCXTypeSignatureCopy allocates a type signature that copies src, which
// lives in the manager, and returns it like AllocCXTypeSignature.
func (cxprogram *CXProgram) AllocCXTypeSignatureCopy(src *CXTypeSignature) (CXTypeSignatureGid, *CXTypeSignature) {
	manager := &cxprogram.CxIrState.CXTypeSignatureMng
	gid := manager.CXTypeSignatureAlloc()
	typeSignature := manager.CXTypeSignatureGet(gid)
	*typeSignature = *src
	typeSignature.Index = CXTypeSignatureIndex(objmodel.GidIndex(int64(gid)))
	return gid, typeSignature
}

// GetCXTypeSignatureFromArray returns the type signature in slot index of
// the program's CXTypeSignatureMng, or nil for -1. A type signature is named
// by its slot index, not its Gid, until phase 5 of CXTypeSignature.
func (cxprogram *CXProgram) GetCXTypeSignatureFromArray(index CXTypeSignatureIndex) *CXTypeSignature {
	if index == -1 {
		return nil
	}

	manager := &cxprogram.CxIrState.CXTypeSignatureMng
	if index < 0 || int(index) >= int(manager.CXTypeSignatureMngNext) {
		panic(fmt.Errorf("error: CXTypeSignature[%d]: index out of bounds", index))
	}

	gid := objmodel.GidCompose(CXTypeSignatureGidTypeTag, manager.CXTypeSignatureMngGenerations[index], uint32(index))
	return manager.CXTypeSignatureGet(CXTypeSignatureGid(gid))
}

// ----------------------------------------------------------------
//
//	`CXProgram` TypeSignature_Array handling

// GetCXTypeSignatureArrayFromArray returns the array signature in slot index
// of the program's CXTypeSignature_ArrayMng, or nil for -1. A type signature's
// Meta keeps the slot index, not the Gid, until phase 5 of
// CXTypeSignature_Array.
func (cxprogram *CXProgram) GetCXTypeSignatureArrayFromArray(index int) *CXTypeSignature_Array {
	if index == -1 {
		return nil
	}

	manager := &cxprogram.CxIrState.CXTypeSignature_ArrayMng
	if index < 0 || index >= int(manager.CXTypeSignature_ArrayMngNext) {
		panic(fmt.Errorf("error: TypeSignatureForArrays[%d]: index out of bounds", index))
	}

	gid := objmodel.GidCompose(CXTypeSignature_ArrayGidTypeTag, manager.CXTypeSignature_ArrayMngGenerations[index], uint32(index))
	return manager.CXTypeSignature_ArrayGet(CXTypeSignature_ArrayGid(gid))
}

// ----------------------------------------------------------------
//
//	`CXProgram` TypeSignature_Struct handling

// GetCXTypeSignatureStructFromArray returns the struct signature in slot index
// of the program's CXTypeSignature_StructMng, or nil for -1. A type
// signature's Meta keeps the slot index, not the Gid, until phase 5 of
// CXTypeSignature_Struct.
func (cxprogram *CXProgram) GetCXTypeSignatureStructFromArray(index int) *CXTypeSignature_Struct {
	if index == -1 {
		return nil
	}

	manager := &cxprogram.CxIrState.CXTypeSignature_StructMng
	if index < 0 || index >= int(manager.CXTypeSignature_StructMngNext) {
		panic(fmt.Errorf("error: TypeSignatureForStructs[%d]: index out of bounds", index))
	}

	gid := objmodel.GidCompose(CXTypeSignature_StructGidTypeTag, manager.CXTypeSignature_StructMngGenerations[index], uint32(index))
	return manager.CXTypeSignature_StructGet(CXTypeSignature_StructGid(gid))
}

// ----------------------------------------------------------------
//
//	`CXProgram` CXArgument handling

// AllocCXArgument allocates an argument in the program's CXArgumentMng, sets
// its Index to the slot and returns its Gid and the pointer to fill it
// through.
func (cxprogram *CXProgram) AllocCXArgument() (CXArgumentGid, *CXArgument) {
	manager := &cxprogram.CxIrState.CXArgumentMng
	gid := manager.CXArgumentAlloc()
	arg := manager.CXArgumentGet(gid)
	arg.Index = int(objmodel.GidIndex(int64(gid)))
	return gid, arg
}

// AddCXArgInArray returns the slot of cxArg. An argument that already lives
// in the program's CXArgumentMng is not copied. A heap argument, which only
// the natives, serialization and tests still make, is copied into a new slot
// and cxArg.Index is set to it.
func (cxprogram *CXProgram) AddCXArgInArray(cxArg *CXArgument) CXArgumentIndex {
	if cxprogram.cxArgumentManaged(cxArg) {
		return CXArgumentIndex(cxArg.Index)
	}

	gid := cxprogram.CxIrState.CXArgumentMng.CXArgumentAlloc()
	cxArg.Index = int(objmodel.GidIndex(int64(gid)))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now an object copy, site S1632.
	*cxprogram.CxIrState.CXArgumentMng.CXArgumentGet(gid) = *cxArg

	return CXArgumentIndex(cxArg.Index)
}

// ReleaseCXArg releases the argument in slot index of the program's
// CXArgumentMng, the counterpart of GetCXArg.
func (cxprogram *CXProgram) ReleaseCXArg(index CXArgumentIndex) {
	manager := &cxprogram.CxIrState.CXArgumentMng
	gid := objmodel.GidCompose(CXArgumentGidTypeTag, manager.CXArgumentMngGenerations[index], uint32(index))
	manager.CXArgumentRelease(CXArgumentGid(gid))
}

// ReleaseCXArgWithDebug releases the argument in slot index and its debug
// record, for an argument that is dropped before anything borrowed the
// record.
func (cxprogram *CXProgram) ReleaseCXArgWithDebug(index CXArgumentIndex) {
	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S1782.
	argDetails := cxprogram.GetCXArg(index).ArgDetails
	cxprogram.ReleaseCXArg(index)
	if argDetails != CXArgumentDebugGidNil {
		cxprogram.CxIrState.CXArgumentDebugMng.CXArgumentDebugRelease(argDetails)
	}
}

// ReleaseCXArgBuilder releases the managed builder cxArg and its debug record,
// when nothing took it. A heap argument is left alone.
func (cxprogram *CXProgram) ReleaseCXArgBuilder(cxArg *CXArgument) {
	if cxprogram.cxArgumentManaged(cxArg) {
		cxprogram.ReleaseCXArgWithDebug(CXArgumentIndex(cxArg.Index))
	}
}

// ReleaseCXArgUnlessNamed releases the managed argument cxArg unless the type
// signature in slot typeSigIdx names it. The signature keeps the debug
// record. A heap argument is left alone.
func (cxprogram *CXProgram) ReleaseCXArgUnlessNamed(cxArg *CXArgument, typeSigIdx CXTypeSignatureIndex) {
	if !cxprogram.cxArgumentManaged(cxArg) {
		return
	}
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
	// Now an arena getter, site S1783.
	typeSig := cxprogram.GetCXTypeSignatureFromArray(typeSigIdx)
	if typeSig.Type != TYPE_CXARGUMENT_DEPRECATE || typeSig.Meta != cxArg.Index {
		cxprogram.ReleaseCXArg(CXArgumentIndex(cxArg.Index))
	}
}

// ReleaseCXTypeSignature releases the type signature in slot index of the
// program's CXTypeSignatureMng, the counterpart of
// GetCXTypeSignatureFromArray.
func (cxprogram *CXProgram) ReleaseCXTypeSignature(index CXTypeSignatureIndex) {
	manager := &cxprogram.CxIrState.CXTypeSignatureMng
	gid := objmodel.GidCompose(CXTypeSignatureGidTypeTag, manager.CXTypeSignatureMngGenerations[index], uint32(index))
	manager.CXTypeSignatureRelease(CXTypeSignatureGid(gid))
}

// cxArgumentManaged reports whether cxArg is the object in its own slot of the
// program's CXArgumentMng, as opposed to an argument on the heap.
func (cxprogram *CXProgram) cxArgumentManaged(cxArg *CXArgument) bool {
	if cxArg == nil {
		return false
	}
	objects := cxprogram.CxIrState.CXArgumentMng.CXArgumentMngObjects
	return cxArg.Index >= 0 && cxArg.Index < len(objects) && &objects[cxArg.Index] == cxArg
}

// cxArgumentSlot returns the slot that a type signature names cxArg by. A
// heap argument is copied in when it has no slot yet, or always when
// copyHeap is set.
func (cxprogram *CXProgram) cxArgumentSlot(cxArg *CXArgument, copyHeap bool) CXArgumentIndex {
	if cxprogram.cxArgumentManaged(cxArg) || !copyHeap && cxArg.Index != -1 && cxArg.Index != 0 {
		return CXArgumentIndex(cxArg.Index)
	}

	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now an arena insert, site S0332.
	return cxprogram.AddCXArgInArray(cxArg)
}

func (cxprogram *CXProgram) GetCXArgFromArray(index CXArgumentIndex) *CXArgument {
	if index == -1 {
		return nil
	}

	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S1633.
	return cxprogram.GetCXArg(index)
}

func (cxprogram *CXProgram) ConvertIndexArgsToPointerArgs(idxs []CXArgumentIndex) []*CXArgument {
	var cxArgs []*CXArgument
	for _, idx := range idxs {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0218.
		arg := cxprogram.GetCXArgFromArray(idx)
		cxArgs = append(cxArgs, arg)
	}
	return cxArgs
}

func (cxprogram *CXProgram) AddPointerArgsToCXArgsArray(cxArgs []*CXArgument) []CXArgumentIndex {
	var cxArgsIdxs []CXArgumentIndex
	for _, cxArg := range cxArgs {
		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now an arena insert, site S0219.
		cxArgIdx := cxprogram.AddCXArgInArray(cxArg)
		cxArgsIdxs = append(cxArgsIdxs, cxArgIdx)
	}
	return cxArgsIdxs
}

// Temporary only for intitial new struct def implementation.
func (cxprogram *CXProgram) ConvertIndexTypeSignaturesToPointerArgs(idxs []CXTypeSignatureIndex) []*CXArgument {
	var cxArgs []*CXArgument
	for _, typeSignatureIdx := range idxs {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0220.
		typeSignature := cxprogram.GetCXTypeSignatureFromArray(typeSignatureIdx)
		if typeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
			idx := typeSignature.Meta
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0221.
			arg := cxprogram.GetCXArgFromArray(CXArgumentIndex(idx))
			cxArgs = append(cxArgs, arg)
		}
	}
	return cxArgs
}

// Temporary only for intitial new struct def implementation.
func (cxprogram *CXProgram) AddPointerArgsToTypeSignaturesArray(cxArgs []*CXArgument) []CXTypeSignatureIndex {
	var cxTypeSignaturesIdxs []CXTypeSignatureIndex
	for _, cxArg := range cxArgs {
		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now an arena insert, site S0222.
		cxArgIdx := cxprogram.AddCXArgInArray(cxArg)

		_, newCXTypeSignature := cxprogram.AllocCXTypeSignature()
		newCXTypeSignature.Name = cxArg.Name
		newCXTypeSignature.Package = cxArg.Package
		newCXTypeSignature.Offset = cxArg.Offset
		newCXTypeSignature.Type = TYPE_CXARGUMENT_DEPRECATE
		newCXTypeSignature.Meta = int(cxArgIdx)
		newCXTypeSignature.ArgDetails = cxArg.ArgDetails
		cxTypeSignaturesIdxs = append(cxTypeSignaturesIdxs, newCXTypeSignature.Index)
	}
	return cxTypeSignaturesIdxs
}

// ----------------------------------------------------------------
//                             `CXProgram` Getters

func (cxprogram *CXProgram) GetOperation(expr *CXExpression) (*CXAtomicOperator, *CXLine, error) {
	switch expr.Type {
	case CX_ATOMIC_OPERATOR:
		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
		// Now an arena getter, site S0225.
		cxAtomicOp, err := cxprogram.GetCXAtomicOp(expr.Index)
		if err != nil {
			// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
			// Now a heap literal, site S0226.
			return &CXAtomicOperator{}, nil, err
		}

		return cxAtomicOp, nil, nil
	case CX_LINE:
		// OBJMODEL: state.CXLineMng.CXLineGet(gid), phase 5 #312.
		// Now an arena getter, site S0231.
		cxLine, err := cxprogram.GetCXLine(expr.Index)
		if err != nil {
			// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
			// Now a heap literal, site S0232.
			return &CXAtomicOperator{}, nil, err
		}

		// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
		// Now a heap literal, site S0235.
		return &CXAtomicOperator{}, cxLine, nil
		// case CX_ARGUMENT:
	}

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
	// Now a heap literal, site S0237.
	return &CXAtomicOperator{}, nil, fmt.Errorf("operation type is not found.")
}

func (cxprogram *CXProgram) GetPreviousCXLine(exprs []CXExpression, currIndex int) (*CXLine, error) {
	// A runtime error while the inputs of an empty function are copied stops at
	// line 0 of a call with no expressions.
	if currIndex >= len(exprs) {
		currIndex = len(exprs) - 1
	}
	for i := currIndex; i >= 0; i-- {
		if exprs[i].Type == CX_LINE {
			_, cxLine, err := cxprogram.GetOperation(&exprs[i])
			if err != nil {
				// OBJMODEL: state.CXLineMng.CXLineAlloc(), phase 4 #311.
				// Now a heap literal, site S0240.
				return &CXLine{}, err
			}

			return cxLine, nil
		}
	}
	// OBJMODEL: state.CXLineMng.CXLineAlloc(), phase 4 #311.
	// Now a heap literal, site S0241.
	return &CXLine{}, fmt.Errorf("CXLine not found.")
}

func (cxprogram *CXProgram) GetCXAtomicOpFromExpressions(exprs []CXExpression, currIndex int) (*CXAtomicOperator, error) {
	for i := currIndex; i < len(exprs); i++ {
		if exprs[i].Type == CX_ATOMIC_OPERATOR {
			// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
			// Now an arena getter, site S0242.
			cxAtomicOp, err := cxprogram.GetCXAtomicOp(exprs[i].Index)
			if err != nil {
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
				// Now a heap literal, site S0243.
				return &CXAtomicOperator{}, err
			}

			return cxAtomicOp, nil
		}
	}
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
	// Now a heap literal, site S0244.
	return &CXAtomicOperator{}, fmt.Errorf("CXAtomicOperator not found.")
}

func (cxprogram *CXProgram) GetPreviousCXAtomicOpFromExpressions(exprs []CXExpression, currIndex int) (*CXAtomicOperator, error) {
	for i := currIndex; i >= 0; i-- {
		if exprs[i].Type == CX_ATOMIC_OPERATOR {
			// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
			// Now an arena getter, site S0245.
			cxAtomicOp, err := cxprogram.GetCXAtomicOp(exprs[i].Index)
			if err != nil {
				// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
				// Now a heap literal, site S0246.
				return &CXAtomicOperator{}, err
			}

			return cxAtomicOp, nil
		}
	}
	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorAlloc(), phase 4 #341.
	// Now a heap literal, site S0247.
	return &CXAtomicOperator{}, fmt.Errorf("CXAtomicOperator not found.")
}

// Only two users, both in cx/execute.go
func (cxprogram *CXProgram) SelectPackage(name stringpool.StringGid) (*CXPackage, error) {
	if _, ok := cxprogram.Packages[name]; !ok {
		return nil, fmt.Errorf("Package '%s' does not exist", cxprogram.CXProgramNameText(name))
	}

	cxprogram.CurrentPackage = cxprogram.Packages[name]

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0248.
	return cxprogram.GetPackageFromArray(cxprogram.Packages[name])
}

// GetCurrentPackage ...
func (cxprogram *CXProgram) GetCurrentPackage() (*CXPackage, error) {
	if cxprogram.CurrentPackage == -1 {
		return nil, errors.New("current package is nil")
	}

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0249.
	return cxprogram.GetPackageFromArray(cxprogram.CurrentPackage)
}

// GetCurrentFunction ...
func (cxprogram *CXProgram) GetCurrentFunction() (*CXFunction, error) {
	if cxprogram.CurrentPackage == -1 {
		return nil, errors.New("current package is nil")
	}

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0250.
	currentPackage, err := cxprogram.GetPackageFromArray(cxprogram.CurrentPackage)
	if err != nil {
		// OBJMODEL: state.CXFunctionMng.CXFunctionAlloc(), phase 4 #351.
		// Now a heap literal, site S0251.
		return &CXFunction{}, err
	}

	if currentPackage.CurrentFunction == -1 {
		return nil, errors.New("current function is nil")
	}

	// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
	// Now an arena getter, site S0252.
	return cxprogram.GetFunctionFromArray(currentPackage.CurrentFunction), nil

}

// GetPackage ...
func (cxprogram *CXProgram) GetPackage(pkgName stringpool.StringGid) (*CXPackage, error) {
	if _, ok := cxprogram.Packages[pkgName]; !ok {
		return nil, fmt.Errorf("package '%s' not found", cxprogram.CXProgramNameText(pkgName))
	}

	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S0253.
	return cxprogram.GetPackageFromArray(cxprogram.Packages[pkgName])
}

// GetStruct ...
func (cxprogram *CXProgram) GetStruct(strctName stringpool.StringGid, pkgName stringpool.StringGid) (*CXStruct, error) {
	pkg, err := cxprogram.GetPackage(pkgName)
	if err != nil {
		return nil, err
	}

	strct, err := pkg.GetStruct(cxprogram, strctName)
	if err != nil {
		return nil, err
	}

	return strct, nil
}

// GetFunction ...
func (cxprogram *CXProgram) GetFunction(fnName stringpool.StringGid, pkgName stringpool.StringGid) (*CXFunction, error) {
	pkg, err := cxprogram.GetPackage(pkgName)
	if err != nil {
		return nil, err
	}

	fn, err := pkg.GetFunction(cxprogram, fnName)
	if err != nil {
		return nil, err
	}

	return fn, nil
}

// GetCurrentCall returns the current CXCall
// TODO: What does this do?
// TODO: Only used in OP_JMP
func (cxprogram *CXProgram) GetCurrentCall() *CXCall {
	return &cxprogram.CallStack[cxprogram.CallCounter]
}

// GetGlobal ...
/*
func (cxprogram *CXProgram) GetGlobal(name string) (*CXArgument, error) {
	mod, err := cxprogram.GetCurrentPackage()
	if err != nil {
		return nil, err
	}

	var foundArgument *CXArgument
	for _, def := range mod.Globals {
		if def.Name == name {
			foundArgument = def
			break
		}
	}

	for _, imp := range mod.Imports {
		for _, def := range imp.Globals {
			if def.Name == name {
				foundArgument = def
				break
			}
		}
	}

	if foundArgument == nil {
		return nil, fmt.Errorf("global '%s' not found", name)
	}
	return foundArgument, nil
}
*/

/*
// GetCurrentOpCode returns the current OpCode
func (cxprogram *CXProgram) GetCurrentOpCode() int {
	return cxprogram.GetCurrentExpression2().Operator.OpCode
}
*/

/*
//not used
func (cxprogram *CXProgram) GetFramePointer() int {
	return cxprogram.GetCurrentCall().FramePointer
}
*/

// ----------------------------------------------------------------
//                             `CXProgram` Debugging

// PrintProgram prints the abstract syntax tree of a CX program in a
// human-readable format.
func (cxprogram *CXProgram) PrintProgram() {
	fmt.Println(ToString(cxprogram))
}

// ProgramStdinInit makes reader the input of read() for the rest of the run.
func (prgrm *CXProgram) ProgramStdinInit(reader io.Reader) {
	prgrm.ProgramStdin = bufio.NewReader(reader)
}
