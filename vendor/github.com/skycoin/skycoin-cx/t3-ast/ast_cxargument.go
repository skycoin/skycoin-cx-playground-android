package ast

import (
	"slices"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t2-globals" // thin: only DBG_GOLANG_STACK_TRACE, FoundCompileErrors, HeapOffset; core globals, keep
)

type CXArgumentIndex int

func (arg CXArgument) IsPointer() bool {
	return arg.Type == types.POINTER
}

func (arg CXArgument) IsString() bool {
	return arg.PointerTargetType == types.STR || arg.Type == types.STR
}

// CXArgumentStringSlicePointer reports whether arg is a *[]str, which takes
// an address as a *[]i32 does instead of the string path of a *str.
func CXArgumentStringSlicePointer(arg *CXArgument) bool {
	return arg.IsPointer() && arg.PointerTargetType == types.STR && arg.IsSlice()
}

// CXArgumentTakesAddress reports whether assigning input to the output element
// out writes the address of input rather than copying its bytes.
func CXArgumentTakesAddress(prgrm *CXProgram, out *CXArgument, input *CXTypeSignature) bool {
	if out.PassBy == constants.PASSBY_REFERENCE {
		return true
	}
	return out.IsPointer() && out.PointerTargetType != types.STR && out.Type != types.AFF && !CXTypeSignaturePointerValue(prgrm, input)
}

// CXTypeSignatureStructTakesAddress reports whether assigning input to the
// struct or pointer struct output out writes the address of input.
func CXTypeSignatureStructTakesAddress(prgrm *CXProgram, out *CXTypeSignature, input *CXTypeSignature) bool {
	if out.Type == TYPE_STRUCT && out.PassBy == constants.PASSBY_REFERENCE {
		return true
	}
	// A field such as b.p is the pointer when it is last.
	pointer := out.Type == TYPE_POINTER_STRUCT && !out.IsDeref
	if fields := prgrm.GetCXTypeSignatureStructFromArray(out.Meta).Fields; len(fields) > 0 {
		pointer = cxArgumentPointerField(prgrm.GetCXArgFromArray(fields[len(fields)-1]))
	}
	return pointer && !CXTypeSignaturePointerValue(prgrm, input)
}

// CXTypeSignaturePointerValue reports whether sig reads a pointer by value, as s
// or pa[0] do, so assigning it copies the pointer. &x and a struct literal do not.
func CXTypeSignaturePointerValue(prgrm *CXProgram, sig *CXTypeSignature) bool {
	if CXTypeSignaturePassBy(prgrm, sig) == constants.PASSBY_REFERENCE {
		return false
	}
	switch sig.Type {
	case TYPE_STRUCT, TYPE_POINTER_STRUCT:
		// A field access such as b.p is decided by its last field. IsDeref of
		// rb.p reads rb, not the field.
		if fields := prgrm.GetCXTypeSignatureStructFromArray(sig.Meta).Fields; len(fields) > 0 {
			return cxArgumentPointerField(prgrm.GetCXArgFromArray(fields[len(fields)-1]).GetAssignmentElement(prgrm))
		}
		return sig.Type == TYPE_POINTER_STRUCT && !sig.IsDeref
	}
	if sig.IsDeref {
		return false
	}
	switch sig.Type {
	case TYPE_POINTER_ATOMIC, TYPE_POINTER_ARRAY_ATOMIC, TYPE_POINTER_SLICE_ATOMIC:
		return true
	case TYPE_CXARGUMENT_DEPRECATE:
		elt := prgrm.GetCXArgFromArray(CXArgumentIndex(sig.Meta)).GetAssignmentElement(prgrm)
		if elt.IsPointer() {
			return true
		}
		// A *i32 keeps the type i32 and ends its specifiers with the pointer. A
		// &T{} literal ends them so too but is a struct.
		specs := elt.DeclarationSpecifiers
		return elt.StructType == nil && len(elt.DereferenceOperations) == 0 && len(specs) > 0 && specs[len(specs)-1] == constants.DECL_POINTER
	}
	return false
}

func (arg CXArgument) IsStruct() bool {
	return arg.StructType != nil
}

func (arg CXArgument) IsSlice() bool {
	if dims := arg.SliceArrayDimensions(); dims > 0 {
		// An element of [][N]T, as a[i] is, is an array.
		return len(arg.Indexes) < dims
	}
	if dims := arg.ArraySliceDimensions(); dims > 0 {
		// An element of [N][]T, as a[i] is, is a slice.
		if len(arg.Indexes) < dims || len(arg.Indexes) >= len(arg.Lengths) {
			return false
		}
		for _, val := range arg.Lengths[len(arg.Indexes):] {
			if val != 0 {
				return false
			}
		}
		return true
	}
	if len(arg.Lengths) > 0 {
		for _, val := range arg.Lengths {
			if val != 0 {
				return false
			}
		}

		return true
	}

	return false
}

/*
	FileName              string
- filename and line number
- can be moved to CX AST annotations (comments to be skipped or map)

	FileLine
*/

/*

Note: PAssBy is not used too many place
Note: Low priority for deprecation
- isnt this same as "pointer"
*/

// ----------------------------------------------------------------
//                             `CXArgument` Getters

// GetAssignmentElement ...
func (arg *CXArgument) GetAssignmentElement(prgrm *CXProgram) *CXArgument {
	if len(arg.Fields) > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0068.
		return prgrm.GetCXArgFromArray(arg.Fields[len(arg.Fields)-1]).GetAssignmentElement(prgrm)
	}
	return arg

}

// GetType ...
func (arg *CXArgument) GetType(prgrm *CXProgram) types.Code {
	fieldCount := len(arg.Fields)
	if fieldCount > 0 {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0069.
		return prgrm.GetCXArgFromArray(arg.Fields[fieldCount-1]).GetType(prgrm)
	}

	if arg.Type == types.POINTER {
		return arg.PointerTargetType
	}
	return arg.Type
}

func (arg *CXArgument) GetSize() types.Pointer {
	if arg.Type != types.I8 {
		return arg.Type.Size()
	}

	return arg.Size
}

// ----------------------------------------------------------------
//                     `CXArgument` Member handling

// SetPackage sets CX package `pkg` of CX argument `arg`.
func (arg *CXArgument) SetPackage(pkg *CXPackage) *CXArgument {
	arg.Package = CXPackageIndex(pkg.Index)
	return arg
}

// SetType ...
func (arg *CXArgument) SetType(typeCode types.Code) *CXArgument {
	arg.Type = typeCode
	size := typeCode.Size()
	arg.Size = size
	arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_BASIC)
	return arg
}

// MakePointer takes an already defined `CXArgument` and turns it into a pointer.
// Only used once, deprecate
// TODO: only used by HTTP, create a better module system
func MakePointer(arg *CXArgument) *CXArgument {
	arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_POINTER)
	arg.PointerTargetType = arg.Type
	arg.Type = types.POINTER
	arg.Size = types.POINTER_SIZE

	return arg
}

// MakeStructParameter helper for creating a struct parameter. It creates a
// `CXArgument` named `argName`, that represents a structure instane of
// `strctName`, from package `pkgName`.
// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
// Now a constructor, site S0072.
func MakeStructParameter(prgrm *CXProgram, pkgName, strctName, argName string) *CXArgument {
	pkg, err := prgrm.GetPackage(prgrm.CXProgramNameIntern(pkgName))
	if err != nil {
		panic(err)
	}

	strct, err := pkg.GetStruct(prgrm, prgrm.CXProgramNameIntern(strctName))
	if err != nil {
		panic(err)
	}

	// Natives are shared by every program, so this argument belongs to none.
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S0073.
	arg := nativeArgument(prgrm.CXProgramNameIntern(argName)).SetType(types.STRUCT)
	// arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_STRUCT)
	arg.Size = strct.GetStructSize(prgrm)
	arg.StructType = strct

	return arg
}

// Slice Helper function for creating parameters for standard library operators.
// The current standard library only uses basic types and slices. If more options are needed, modify this function
// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
// Now a constructor, site S0074.
func Slice(typeCode types.Code) *CXArgument {
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S0075.
	arg := Param(typeCode)
	arg.DeclarationSpecifiers = append(arg.DeclarationSpecifiers, constants.DECL_SLICE)
	return arg
}

// Func Helper function for creating function parameters for standard library operators.
// The current standard library only uses basic types and slices. If more options are needed, modify this function
// func Func(pkg *CXPackage, inputs []*CXArgument, outputs []*CXArgument) *CXArgument {
// 	arg := Param(types.FUNC)
// 	arg.Package = CXPackageIndex(pkg.Index)
// 	arg.Inputs = inputs
// 	arg.Outputs = outputs
// 	return arg
// }

// Param ...
// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
// Now a constructor, site S0076.
func Param(typeCode types.Code) *CXArgument {
	// Natives are shared by every program, so this argument belongs to none.
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S0077.
	arg := nativeArgument(stringpool.StringGidNil).SetType(typeCode)
	// arg.IsLocalDeclaration = true
	return arg
}

// nativeArgument makes an argument for a native. The natives are shared by
// every program, so it lives on the heap, belongs to none and has no debug
// record.
// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
// Now a constructor, site S0078.
func nativeArgument(name stringpool.StringGid) *CXArgument {
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a heap literal, site S0079.
	return &CXArgument{
		Name:    name,
		Package: -1,
	}
}

// MakeArgument allocates an argument named name in the CXArgumentMng of
// prgrm, with a debug record for fileName and fileLine, and returns its Gid
// and the pointer to fill it through.
func MakeArgument(prgrm *CXProgram, name stringpool.StringGid, fileName SourceFileGid, fileLine int) (CXArgumentGid, *CXArgument) {
	argDetailsGid := argumentDebugAlloc(prgrm, fileName, fileLine)
	gid, arg := prgrm.AllocCXArgument()
	arg.Name = name
	arg.Package = -1
	arg.ArgDetails = argDetailsGid
	return gid, arg
}

// MakeField allocates an argument of type typeCode like MakeArgument.
func MakeField(prgrm *CXProgram, name stringpool.StringGid, typeCode types.Code, fileName SourceFileGid, fileLine int) (CXArgumentGid, *CXArgument) {
	gid, arg := MakeArgument(prgrm, name, fileName, fileLine)
	arg.Type = typeCode
	return gid, arg
}

// MakeGlobal allocates a global of type typeCode like MakeArgument and gives
// it the next offset in the heap segment.
func MakeGlobal(prgrm *CXProgram, name stringpool.StringGid, typeCode types.Code, fileName SourceFileGid, fileLine int) (CXArgumentGid, *CXArgument) {
	gid, global := MakeField(prgrm, name, typeCode, fileName, fileLine)
	global.Size = typeCode.Size()
	global.Offset = globals.HeapOffset
	globals.HeapOffset += global.Size
	return gid, global
}

// ------------------------------------------------------------------------------------------
//           Special functions to determine its type (atomic, array etomic, etc)

func IsTypeAtomic(arg *CXArgument) bool {
	// TODO: implement including types.IDENTIFIER
	// return (arg.Type.IsPrimitive() || arg.Type == types.IDENTIFIER) && !arg.IsSlice() && len(arg.Lengths) == 0 && len(arg.Fields) == 0 && len(arg.DereferenceOperations) == 0 && (len(arg.DeclarationSpecifiers) == 0 || (len(arg.DeclarationSpecifiers) == 1 && arg.DeclarationSpecifiers[0] == constants.DECL_BASIC))

	return arg.Type.IsPrimitive() && !arg.IsSlice() && len(arg.Lengths) == 0 && len(arg.Fields) == 0 && len(arg.DereferenceOperations) == 0 && (len(arg.DeclarationSpecifiers) == 0 || (len(arg.DeclarationSpecifiers) == 1 && arg.DeclarationSpecifiers[0] == constants.DECL_BASIC))
}

func IsTypePointerAtomic(arg *CXArgument) bool {
	return arg.Type.IsPrimitive() && arg.PointerTargetType == 0 && arg.StructType == nil && !arg.IsSlice() && len(arg.Lengths) == 0 && len(arg.Fields) == 0 && len(arg.DereferenceOperations) == 0 && (len(arg.DeclarationSpecifiers) == 2 && arg.DeclarationSpecifiers[0] == constants.DECL_BASIC && arg.DeclarationSpecifiers[1] == constants.DECL_POINTER)
}

func IsTypeArrayAtomic(arg *CXArgument) bool {
	isThereDeclPointer := false
	for _, decl := range arg.DeclarationSpecifiers {
		if decl == constants.DECL_POINTER {
			isThereDeclPointer = true
		}
	}

	return !isThereDeclPointer && arg.Type.IsPrimitive() && !arg.IsSlice() && len(arg.Lengths) > 0 && !slices.Contains(arg.Lengths, 0) && len(arg.Fields) == 0 && len(arg.DereferenceOperations) == 0 && (len(arg.DeclarationSpecifiers) >= 2 && arg.DeclarationSpecifiers[0] == constants.DECL_BASIC && arg.DeclarationSpecifiers[1] == constants.DECL_ARRAY)
}

func IsTypePointerArrayAtomic(arg *CXArgument) bool {
	isThereDeclPointer := false
	for _, decl := range arg.DeclarationSpecifiers {
		if decl == constants.DECL_POINTER {
			isThereDeclPointer = true
		}
	}

	return isThereDeclPointer && arg.Type.IsPrimitive() && !arg.IsSlice() && len(arg.Lengths) > 0 && !slices.Contains(arg.Lengths, 0) && len(arg.Fields) == 0 && len(arg.DereferenceOperations) == 0 && (len(arg.DeclarationSpecifiers) >= 2 && arg.DeclarationSpecifiers[0] == constants.DECL_BASIC && arg.DeclarationSpecifiers[1] == constants.DECL_ARRAY)
}

func IsTypeSliceAtomic(arg *CXArgument) bool {
	isThereDeclPointer := false
	for _, decl := range arg.DeclarationSpecifiers {
		if decl == constants.DECL_POINTER {
			isThereDeclPointer = true
		}
	}

	return !isThereDeclPointer && arg.Type.IsPrimitive() && arg.IsSlice() && len(arg.Lengths) > 0 && len(arg.Fields) == 0 && len(arg.DereferenceOperations) == 0 && (len(arg.DeclarationSpecifiers) >= 2 && arg.DeclarationSpecifiers[0] == constants.DECL_BASIC && arg.DeclarationSpecifiers[1] == constants.DECL_SLICE)
}

func IsTypePointerSliceAtomic(arg *CXArgument) bool {
	isThereDeclPointer := false
	for _, decl := range arg.DeclarationSpecifiers {
		if decl == constants.DECL_POINTER {
			isThereDeclPointer = true
		}
	}

	return isThereDeclPointer && arg.Type.IsPrimitive() && arg.IsSlice() && len(arg.Lengths) > 0 && len(arg.Fields) == 0 && len(arg.DereferenceOperations) == 0 && (len(arg.DeclarationSpecifiers) >= 2 && arg.DeclarationSpecifiers[0] == constants.DECL_BASIC && arg.DeclarationSpecifiers[1] == constants.DECL_SLICE)
}

func IsTypeStruct(arg *CXArgument) bool {
	isThereDeclPointer := false
	IsThereDeclSlice := false
	for _, decl := range arg.DeclarationSpecifiers {
		if decl == constants.DECL_POINTER {
			isThereDeclPointer = true
		}

		if decl == constants.DECL_SLICE {
			IsThereDeclSlice = true
		}
	}

	return !isThereDeclPointer && !IsThereDeclSlice && !arg.IsSlice() && len(arg.Lengths) == 0 && arg.StructType != nil && len(arg.DeclarationSpecifiers) >= 1 && arg.DeclarationSpecifiers[0] == constants.DECL_STRUCT
}

func IsTypePointerStruct(arg *CXArgument) bool {
	isThereDeclPointer := false
	IsThereDeclSlice := false
	for _, decl := range arg.DeclarationSpecifiers {
		if decl == constants.DECL_POINTER {
			isThereDeclPointer = true
		}

		if decl == constants.DECL_SLICE {
			IsThereDeclSlice = true
		}
	}

	return isThereDeclPointer && !IsThereDeclSlice && !arg.IsSlice() && len(arg.Lengths) == 0 && arg.StructType != nil && len(arg.DeclarationSpecifiers) >= 1 && arg.DeclarationSpecifiers[0] == constants.DECL_STRUCT
}

// argumentDebugAlloc allocates the debug record of an argument in prgrm's
// CXArgumentDebugMng. A nil prgrm, for a native, gets CXArgumentDebugGidNil.
func argumentDebugAlloc(prgrm *CXProgram, fileName SourceFileGid, fileLine int) CXArgumentDebugGid {
	if prgrm == nil {
		return CXArgumentDebugGidNil
	}
	manager := &prgrm.CxIrState.CXArgumentDebugMng
	gid := manager.CXArgumentDebugAlloc()
	details := manager.CXArgumentDebugGet(gid)
	details.FileName = fileName
	details.FileLine = fileLine
	details.FileColumn = prgrm.CXProgramCompileErrorColumn(fileName, fileLine)
	return gid
}

// CXArgumentDebugFileLine returns the file name and line that gid names in
// prgrm, the name as text for diagnostics. A native parameter has none and
// gets "" and -1, as Param gave it.
func CXArgumentDebugFileLine(prgrm *CXProgram, gid CXArgumentDebugGid) (string, int) {
	if gid == CXArgumentDebugGidNil {
		return "", -1
	}
	details := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(gid)
	return prgrm.CXProgramSourceFileText(details.FileName), details.FileLine
}

// cxArgumentPointerField reports whether the field access arg is a pointer,
// also a *i32, which keeps the type i32 and ends its specifiers with it.
func cxArgumentPointerField(arg *CXArgument) bool {
	if arg.IsPointer() {
		return true
	}
	specs := arg.DeclarationSpecifiers
	return arg.StructType == nil && len(arg.DereferenceOperations) == 0 && len(arg.Indexes) == 0 && len(specs) > 0 && specs[len(specs)-1] == constants.DECL_POINTER
}

// ArraySliceDimensions returns the number of array dimensions of an array
// of slices, 1 for [N][]T, and 0 for any other type.
func (arg CXArgument) ArraySliceDimensions() int {
	for i, length := range arg.Lengths {
		if length == 0 {
			return i
		}
	}
	return 0
}

// SliceArrayDimensions returns the number of slice dimensions of a slice of
// arrays, 1 for [][N]T, and 0 for any other type.
func (arg CXArgument) SliceArrayDimensions() int {
	for i, length := range arg.Lengths {
		if length != 0 {
			return i
		}
	}
	return 0
}

// ValueSizeOfLengths returns the size of a value of the array and slice
// lengths, outermost first, whose innermost elements take elem bytes.
func ValueSizeOfLengths(lengths []types.Pointer, elem types.Pointer) types.Pointer {
	size := elem
	for i := len(lengths) - 1; i >= 0; i-- {
		if lengths[i] == 0 {
			size = types.POINTER_SIZE
		} else {
			size *= lengths[i]
		}
	}
	return size
}

// ArraySize returns the size in bytes of the array arg, whose elements are
// slice headers when it is an array of slices.
func (arg CXArgument) ArraySize() types.Pointer {
	if dims := arg.ArraySliceDimensions(); dims > 0 {
		return types.POINTER_SIZE * types.TotalLength(arg.Lengths[:dims])
	}
	return arg.Size * types.TotalLength(arg.Lengths)
}
