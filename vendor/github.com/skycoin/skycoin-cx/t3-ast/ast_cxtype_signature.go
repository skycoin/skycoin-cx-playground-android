package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
)

// CXTypeSignature_TYPE enum contains CXTypeSignature types.
type CXTypeSignature_TYPE int

type CXTypeSignatureIndex int

const (
	TYPE_UNUSED CXTypeSignature_TYPE = iota
	TYPE_ATOMIC
	TYPE_POINTER_ATOMIC
	TYPE_ARRAY_ATOMIC
	TYPE_POINTER_ARRAY_ATOMIC
	TYPE_SLICE_ATOMIC
	TYPE_POINTER_SLICE_ATOMIC

	TYPE_STRUCT
	TYPE_POINTER_STRUCT
	TYPE_ARRAY_STRUCT
	TYPE_POINTER_ARRAY_STRUCT
	TYPE_SLICE_STRUCT
	TYPE_SLICE_POINTER_STRUCT

	TYPE_COMPLEX
	TYPE_POINTER_COMPLEX
	TYPE_ARRAY_COMPLEX
	TYPE_ARRAY_POINTER_COMPLEX
	TYPE_SLICE_COMPLEX
	TYPE_SLICE_POINTER_COMPLEX

	// For CXArgument usage
	// To be deprecated
	TYPE_CXARGUMENT_DEPRECATE
)

// ----------------------------------------------------------------
//                             `CXTypeSignature` Handlers

func (typeSignature *CXTypeSignature) GetSize(prgrm *CXProgram, IsForUpdateSymbolsTable bool) types.Pointer {
	switch typeSignature.Type {
	case TYPE_ATOMIC:
		return types.Code(typeSignature.Meta).Size()
	case TYPE_POINTER_ATOMIC:
		if typeSignature.IsDeref {
			return types.Code(typeSignature.Meta).Size()
		}

		return types.POINTER.Size()
	case TYPE_ARRAY_ATOMIC:
		if IsForUpdateSymbolsTable {
			return typeSignature.GetArraySize(prgrm)
		}

		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0273.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		if len(arrDetails.Indexes) > 0 {
			return indexedValueSize(arrDetails)
		}

		return typeSignature.GetArraySize(prgrm)
	case TYPE_POINTER_ARRAY_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0274.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		if typeSignature.IsDeref && len(arrDetails.Indexes) > 0 {
			return indexedValueSize(arrDetails)
		} else if typeSignature.IsDeref {
			return typeSignature.GetArraySize(prgrm)
		}

		return types.POINTER.Size()
	case TYPE_SLICE_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0275.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)

		if len(sliceDetails.Indexes) > 0 {
			return indexedValueSize(sliceDetails)
		}
		if typeSignature.IsDeref {
			return types.Code(sliceDetails.Meta).Size()
		}

		return types.POINTER_SIZE
	case TYPE_POINTER_SLICE_ATOMIC:

		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0276.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		if len(sliceDetails.Indexes) > 0 {
			return indexedValueSize(sliceDetails)
		}

		// *p without an index is the slice header pointer, like p itself.
		return types.POINTER.Size()
	case TYPE_STRUCT:
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0277.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)

		lenFlds := len(structDetails.Fields)
		if lenFlds > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0278.
			return GetArgSize(prgrm, prgrm.GetCXArg(CXArgumentIndex(structDetails.Fields[lenFlds-1])))
		}
		return structDetails.StructType.GetStructSize(prgrm)
	case TYPE_POINTER_STRUCT:
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0279.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)

		lenFlds := len(structDetails.Fields)
		if lenFlds > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0280.
			return GetArgSize(prgrm, prgrm.GetCXArg(CXArgumentIndex(structDetails.Fields[lenFlds-1])))
		}

		if typeSignature.IsDeref {
			return structDetails.StructType.GetStructSize(prgrm)
		}

		return types.POINTER.Size()
	case TYPE_ARRAY_STRUCT:
	case TYPE_POINTER_ARRAY_STRUCT:
	case TYPE_SLICE_STRUCT:
	case TYPE_SLICE_POINTER_STRUCT:

	case TYPE_COMPLEX:
	case TYPE_POINTER_COMPLEX:
	case TYPE_ARRAY_COMPLEX:
	case TYPE_ARRAY_POINTER_COMPLEX:
	case TYPE_SLICE_COMPLEX:
	case TYPE_SLICE_POINTER_COMPLEX:

	case TYPE_CXARGUMENT_DEPRECATE:
		argIdx := typeSignature.Meta
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0281.
		return GetArgSize(prgrm, prgrm.GetCXArg(CXArgumentIndex(argIdx)))
	}

	return 0
}

func (typeSignature *CXTypeSignature) GetArrayLength(prgrm *CXProgram) types.Pointer {
	switch typeSignature.Type {
	case TYPE_ARRAY_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0282.
		typeSignatureForArray := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		return types.TotalLength(typeSignatureForArray.Lengths)
	}

	return 0
}

func (typeSignature *CXTypeSignature) GetArraySize(prgrm *CXProgram) types.Pointer {
	switch typeSignature.Type {
	case TYPE_ARRAY_ATOMIC, TYPE_POINTER_ARRAY_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0283.
		typeSignatureForArray := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		arrType := types.Code(typeSignatureForArray.Meta)
		return types.TotalLength(typeSignatureForArray.Lengths) * arrType.Size()
	}

	return 0
}

func (typeSignature *CXTypeSignature) GetType(prgrm *CXProgram) types.Code {
	var atomicType types.Code
	switch typeSignature.Type {
	case TYPE_ATOMIC:
		atomicType = types.Code(typeSignature.Meta)
	case TYPE_POINTER_ATOMIC:
		atomicType = types.Code(typeSignature.Meta)
	case TYPE_ARRAY_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0284.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		atomicType = types.Code(arrDetails.Meta)
	case TYPE_POINTER_ARRAY_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0285.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		atomicType = types.Code(arrDetails.Meta)
	case TYPE_SLICE_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0286.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		atomicType = types.Code(sliceDetails.Meta)
	case TYPE_POINTER_SLICE_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0287.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		atomicType = types.Code(sliceDetails.Meta)
	case TYPE_STRUCT:
		atomicType = types.STRUCT

		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0288.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
		fldLen := len(structDetails.Fields)
		if fldLen > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0289.
			fld := prgrm.GetCXArgFromArray(structDetails.Fields[fldLen-1])
			fld = fld.GetAssignmentElement(prgrm)

			atomicType = fld.Type
			if fld.Type == types.POINTER {
				atomicType = fld.PointerTargetType
			}
		}
	case TYPE_POINTER_STRUCT:
		atomicType = types.STRUCT

		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0290.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
		fldLen := len(structDetails.Fields)
		if fldLen > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0291.
			fld := prgrm.GetCXArgFromArray(structDetails.Fields[fldLen-1])
			fld = fld.GetAssignmentElement(prgrm)

			atomicType = fld.Type
			if fld.Type == types.POINTER {
				atomicType = fld.PointerTargetType
			}
		}
	default:
		panic("type is not known")
	}

	return atomicType
}

func (typeSignature *CXTypeSignature) GetTypeString(prgrm *CXProgram) string {
	var typeString string

	switch typeSignature.Type {
	case TYPE_ATOMIC:
		typeString = types.Code(typeSignature.Meta).Name()
	case TYPE_POINTER_ATOMIC:
		typeString = types.Code(typeSignature.Meta).Name()
	case TYPE_ARRAY_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0292.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		typeString = types.Code(arrDetails.Meta).Name()
	case TYPE_POINTER_ARRAY_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0293.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		typeString = types.Code(arrDetails.Meta).Name()
	case TYPE_SLICE_ATOMIC, TYPE_POINTER_SLICE_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0294.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		typeString = types.Code(arrDetails.Meta).Name()
	case TYPE_STRUCT:
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0295.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
		fldsLen := len(structDetails.Fields)
		if fldsLen > 0 {
			fldIdx := structDetails.Fields[fldsLen-1]
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0296.
			fld := prgrm.GetCXArgFromArray(fldIdx)
			elt := fld.GetAssignmentElement(prgrm)

			if elt.StructType != nil {
				// then it's custom type
				typeString = prgrm.CXProgramNameText(elt.StructType.Name)
			} else {
				// then it's native type
				typeString = elt.Type.Name()

				if elt.Type == types.POINTER {
					typeString = elt.PointerTargetType.Name()
				}
			}
		} else {
			typeString = prgrm.CXProgramNameText(structDetails.StructType.Name)
		}

	case TYPE_POINTER_STRUCT:
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0297.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
		fldsLen := len(structDetails.Fields)
		if fldsLen > 0 {
			fldIdx := structDetails.Fields[fldsLen-1]
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0298.
			fld := prgrm.GetCXArgFromArray(fldIdx)
			elt := fld.GetAssignmentElement(prgrm)

			if elt.StructType != nil {
				// then it's custom type
				typeString = prgrm.CXProgramNameText(elt.StructType.Name)
			} else {
				// then it's native type
				typeString = elt.Type.Name()

				if elt.Type == types.POINTER {
					typeString = elt.PointerTargetType.Name()
				}
			}
		} else {
			typeString = prgrm.CXProgramNameText(structDetails.StructType.Name)
		}
	default:
		panic("type is not known")

	}

	return typeString
}

// GetValueTypeName returns the type name printf gives an extra value, the
// struct name for a struct and the element type name otherwise.
func (typeSignature *CXTypeSignature) GetValueTypeName(prgrm *CXProgram) string {
	switch typeSignature.Type {
	case TYPE_CXARGUMENT_DEPRECATE:
		element := prgrm.GetCXArgFromArray(CXArgumentIndex(typeSignature.Meta)).GetAssignmentElement(prgrm)
		if element.StructType != nil {
			return prgrm.CXProgramNameText(element.StructType.Name)
		}
		return element.Type.Name()
	case TYPE_ATOMIC, TYPE_POINTER_ATOMIC, TYPE_ARRAY_ATOMIC, TYPE_POINTER_ARRAY_ATOMIC,
		TYPE_SLICE_ATOMIC, TYPE_POINTER_SLICE_ATOMIC, TYPE_STRUCT, TYPE_POINTER_STRUCT:
		return typeSignature.GetTypeString(prgrm)
	}
	return types.UNDEFINED.Name()
}

func (typeSignature *CXTypeSignature) GetCXArgFormat(prgrm *CXProgram) *CXArgument {
	// MISUSE: builds an IR argument for the garbage collector, which needs only
	// the offset, types and declaration specifiers of a field, see #423.
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a heap literal, site S0299.
	var arg *CXArgument = &CXArgument{}
	if typeSignature.Type == TYPE_ATOMIC {
		arg.Type = types.Code(typeSignature.Meta)
		arg.StructType = nil
		arg.Size = typeSignature.GetSize(prgrm, false)

		// TODO: this should not be needed.
		if len(arg.DeclarationSpecifiers) > 0 {
			arg.DeclarationSpecifiers = append([]int{constants.DECL_BASIC}, arg.DeclarationSpecifiers[1:]...)
		} else {
			arg.DeclarationSpecifiers = []int{constants.DECL_BASIC}
		}

	} else if typeSignature.Type == TYPE_POINTER_ATOMIC {
		arg.Type = types.POINTER
		arg.PointerTargetType = types.Code(typeSignature.Meta)
		arg.StructType = nil
		arg.Size = types.POINTER.Size()

		// TODO: this should not be needed.
		if len(arg.DeclarationSpecifiers) > 0 {
			arg.DeclarationSpecifiers = append([]int{constants.DECL_BASIC, constants.DECL_POINTER}, arg.DeclarationSpecifiers[1:]...)
		} else {
			arg.DeclarationSpecifiers = []int{constants.DECL_POINTER}
		}
	} else if typeSignature.Type == TYPE_ARRAY_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0300.
		typeSignatureArray := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		arg.Type = types.Code(typeSignatureArray.Meta)
		arg.StructType = nil
		arg.Size = typeSignature.GetSize(prgrm, false)
		arg.Lengths = typeSignatureArray.Lengths
		arg.Indexes = typeSignatureArray.Indexes

		// TODO: this should not be needed.
		if len(arg.DeclarationSpecifiers) > 0 {
			arg.DeclarationSpecifiers = append([]int{constants.DECL_BASIC, constants.DECL_ARRAY}, arg.DeclarationSpecifiers[1:]...)
		} else {
			arg.DeclarationSpecifiers = []int{constants.DECL_BASIC}
		}

	} else if typeSignature.Type == TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0301.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(typeSignature.Meta)
		arg.Type = types.Code(sliceDetails.Meta)
		arg.StructType = nil
		arg.Size = types.Code(sliceDetails.Meta).Size()
		arg.Lengths = sliceDetails.Lengths

		// TODO: this should not be needed.
		if len(arg.DeclarationSpecifiers) > 0 {
			arg.DeclarationSpecifiers = append([]int{constants.DECL_BASIC, constants.DECL_SLICE}, arg.DeclarationSpecifiers[1:]...)
		} else {
			arg.DeclarationSpecifiers = []int{constants.DECL_BASIC}
		}

		arg.DereferenceOperations = append([]int{constants.DEREF_POINTER}, arg.DereferenceOperations...)

	} else if typeSignature.Type == TYPE_STRUCT {
		arg.Type = types.STRUCT
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0302.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
		arg.StructType = structDetails.StructType
		// TODO: this should not be needed.
		if len(arg.DeclarationSpecifiers) > 0 {
			arg.DeclarationSpecifiers = append([]int{constants.DECL_STRUCT}, arg.DeclarationSpecifiers[1:]...)
		} else {
			arg.DeclarationSpecifiers = []int{constants.DECL_STRUCT}
		}

	} else if typeSignature.Type == TYPE_POINTER_STRUCT {
		arg.Type = types.POINTER
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0303.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta)
		arg.StructType = structDetails.StructType
		// TODO: this should not be needed.
		if len(arg.DeclarationSpecifiers) > 0 {
			arg.DeclarationSpecifiers = append([]int{constants.DECL_STRUCT, constants.DECL_POINTER}, arg.DeclarationSpecifiers[1:]...)
		} else {
			arg.DeclarationSpecifiers = []int{constants.DECL_STRUCT, constants.DECL_POINTER}
		}

	}
	arg.Offset = typeSignature.Offset

	return arg
}

// IsInnerReference determines if it is an inner reference
// for example: &slice[0] or &struct.field
func (typeSignature *CXTypeSignature) IsInnerReference(prgrm *CXProgram) bool {
	switch typeSignature.Type {
	case TYPE_CXARGUMENT_DEPRECATE:
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0304.
		arg := prgrm.GetCXArgFromArray(CXArgumentIndex(typeSignature.Meta))

		if arg.PassBy == constants.PASSBY_REFERENCE && len(arg.Indexes) > 0 {
			for _, decl := range arg.DeclarationSpecifiers {
				if decl == constants.DECL_POINTER {
					return true
				}
			}

		}
	}

	return false
}

// fillCXTypeSignatureOfCXArgForStructs fills newCXTypeSignature from the struct
// field cxArgument. It returns false when the field has no signature type of
// its own, and the caller makes the signature name the argument.
// Added _ForStructs for now so it will be separate from others for smooth transition.
// TODO: Remove _ForStructs
func fillCXTypeSignatureOfCXArgForStructs(prgrm *CXProgram, newCXTypeSignature *CXTypeSignature, cxArgument *CXArgument) bool {
	newCXTypeSignature.Name = cxArgument.Name
	newCXTypeSignature.Package = cxArgument.Package
	newCXTypeSignature.PassBy = cxArgument.PassBy
	newCXTypeSignature.ArgDetails = cxArgument.ArgDetails

	fieldType := cxArgument.Type
	if IsTypeAtomic(cxArgument) {
		// If atomic type. i.e. i8, i16, i32, f32, etc.

		newCXTypeSignature.Type = TYPE_ATOMIC
		newCXTypeSignature.Meta = int(fieldType)

	} else if IsTypePointerAtomic(cxArgument) {
		// If pointer atomic, i.e. *i32, *f32, etc.

		newCXTypeSignature.Type = TYPE_POINTER_ATOMIC
		newCXTypeSignature.Offset = cxArgument.Offset

		newCXTypeSignature.Meta = int(cxArgument.Type)
		if cxArgument.Type == types.STR || cxArgument.StructType != nil {
			newCXTypeSignature.Meta = int(cxArgument.PointerTargetType)
		}

	} else if IsTypeArrayAtomic(cxArgument) {
		// If simple array atomic type, i.e. [5]i32, [2]f64, etc.

		newCXTypeSignature.Type = TYPE_ARRAY_ATOMIC

		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		typeSignatureForArray.Meta = int(fieldType)
		typeSignatureForArray.Lengths = cxArgument.Lengths
		typeSignatureForArray.Indexes = cxArgument.Indexes

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(arrayGid)))

	} else if IsTypePointerArrayAtomic(cxArgument) {
		// If pointer array atomic type, i.e. *[5]i32, *[2]f64, etc.

		newCXTypeSignature.Type = TYPE_POINTER_ARRAY_ATOMIC

		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		typeSignatureForArray.Meta = int(fieldType)
		typeSignatureForArray.Lengths = cxArgument.Lengths
		typeSignatureForArray.Indexes = cxArgument.Indexes

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(arrayGid)))

	} else if IsTypeSliceAtomic(cxArgument) {
		// If slice atomic type, i.e. []i32, []f64, etc.

		newCXTypeSignature.Type = TYPE_SLICE_ATOMIC

		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		typeSignatureForArray.Meta = int(cxArgument.Type)
		typeSignatureForArray.Lengths = cxArgument.Lengths
		typeSignatureForArray.Indexes = cxArgument.Indexes

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(arrayGid)))
	} else if IsTypePointerSliceAtomic(cxArgument) {
		// If pointer slice atomic type, i.e. *[]i32, *[]f64, etc.

		newCXTypeSignature.Type = TYPE_POINTER_SLICE_ATOMIC

		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		typeSignatureForArray.Meta = int(cxArgument.Type)
		typeSignatureForArray.Lengths = cxArgument.Lengths
		typeSignatureForArray.Indexes = cxArgument.Indexes

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(arrayGid)))
	} else if IsTypeStruct(cxArgument) {
		// If type is struct
		newCXTypeSignature.Type = TYPE_STRUCT

		structGid := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructAlloc()
		typeSignatureForStruct := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructGet(structGid)
		typeSignatureForStruct.Fields = cxArgument.Fields
		typeSignatureForStruct.StructType = cxArgument.StructType

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(structGid)))
	} else if IsTypePointerStruct(cxArgument) {
		// If type is struct
		newCXTypeSignature.Type = TYPE_POINTER_STRUCT

		structGid := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructAlloc()
		typeSignatureForStruct := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructGet(structGid)
		typeSignatureForStruct.Fields = cxArgument.Fields
		typeSignatureForStruct.StructType = cxArgument.StructType

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(structGid)))
	} else {
		return false
	}
	return true
}

// fillCXTypeSignatureOfCXArg fills newCXTypeSignature from cxArgument. It
// returns false when the argument has no signature type of its own, and the
// caller makes the signature name the argument.
func fillCXTypeSignatureOfCXArg(prgrm *CXProgram, newCXTypeSignature *CXTypeSignature, cxArgument *CXArgument) bool {
	newCXTypeSignature.Name = cxArgument.Name
	newCXTypeSignature.Package = cxArgument.Package
	newCXTypeSignature.PassBy = cxArgument.PassBy
	newCXTypeSignature.ArgDetails = cxArgument.ArgDetails

	if IsTypeAtomic(cxArgument) {
		// If atomic type. i.e. i8, i16, i32, f32, etc.
		newCXTypeSignature.Type = TYPE_ATOMIC
		newCXTypeSignature.Meta = int(cxArgument.Type)
		newCXTypeSignature.Offset = cxArgument.Offset
	} else if IsTypePointerAtomic(cxArgument) {
		// If pointer atomic, i.e. *i32, *f32, etc.
		newCXTypeSignature.Type = TYPE_POINTER_ATOMIC
		newCXTypeSignature.Offset = cxArgument.Offset

		newCXTypeSignature.Meta = int(cxArgument.Type)
		if cxArgument.Type == types.STR || cxArgument.StructType != nil {
			newCXTypeSignature.Meta = int(cxArgument.PointerTargetType)
		}
	} else if IsTypeArrayAtomic(cxArgument) {
		// If simple array atomic type, i.e. [5]i32, [2]f64, etc.
		newCXTypeSignature.Type = TYPE_ARRAY_ATOMIC

		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		typeSignatureForArray.Meta = int(cxArgument.Type)
		typeSignatureForArray.Lengths = cxArgument.Lengths
		typeSignatureForArray.Indexes = cxArgument.Indexes

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(arrayGid)))
	} else if IsTypePointerArrayAtomic(cxArgument) {
		// If pointer array atomic type, i.e. *[5]i32, *[2]f64, etc.

		newCXTypeSignature.Type = TYPE_POINTER_ARRAY_ATOMIC

		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		typeSignatureForArray.Meta = int(cxArgument.Type)
		typeSignatureForArray.Lengths = cxArgument.Lengths
		typeSignatureForArray.Indexes = cxArgument.Indexes

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(arrayGid)))
	} else if IsTypeSliceAtomic(cxArgument) {
		// If slice atomic type, i.e. []i32, []f64, etc.

		newCXTypeSignature.Type = TYPE_SLICE_ATOMIC

		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		typeSignatureForArray.Meta = int(cxArgument.Type)
		typeSignatureForArray.Lengths = cxArgument.Lengths
		typeSignatureForArray.Indexes = cxArgument.Indexes

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(arrayGid)))
	} else if IsTypePointerSliceAtomic(cxArgument) {
		// If pointer slice atomic type, i.e. *[]i32, *[]f64, etc.

		newCXTypeSignature.Type = TYPE_POINTER_SLICE_ATOMIC

		arrayGid := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayAlloc()
		typeSignatureForArray := prgrm.CxIrState.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(arrayGid)
		typeSignatureForArray.Meta = int(cxArgument.Type)
		typeSignatureForArray.Lengths = cxArgument.Lengths
		typeSignatureForArray.Indexes = cxArgument.Indexes

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(arrayGid)))
	} else if IsTypeStruct(cxArgument) {
		// If type is struct
		newCXTypeSignature.Type = TYPE_STRUCT

		structGid := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructAlloc()
		typeSignatureForStruct := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructGet(structGid)
		typeSignatureForStruct.Fields = cxArgument.Fields
		typeSignatureForStruct.StructType = cxArgument.StructType

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(structGid)))
	} else if IsTypePointerStruct(cxArgument) {
		// If type is struct
		newCXTypeSignature.Type = TYPE_POINTER_STRUCT

		structGid := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructAlloc()
		typeSignatureForStruct := prgrm.CxIrState.CXTypeSignature_StructMng.CXTypeSignature_StructGet(structGid)
		typeSignatureForStruct.Fields = cxArgument.Fields
		typeSignatureForStruct.StructType = cxArgument.StructType

		newCXTypeSignature.Meta = int(objmodel.GidIndex(int64(structGid)))
	} else {
		return false
	}
	return true
}

// AllocCXTypeSignatureOfCXArgForStructs allocates the signature of the struct
// field cxArgument in the program's CXTypeSignatureMng and returns its slot.
// A field with no signature type of its own is named by the signature, and a
// heap field is copied in first.
func AllocCXTypeSignatureOfCXArgForStructs(prgrm *CXProgram, cxArgument *CXArgument) CXTypeSignatureIndex {
	_, newCXTypeSignature := prgrm.AllocCXTypeSignature()
	if !fillCXTypeSignatureOfCXArgForStructs(prgrm, newCXTypeSignature, cxArgument) {
		// All are TYPE_CXARGUMENT_DEPRECATE for now. The CXArg slot is in Meta.
		newCXTypeSignature.Type = TYPE_CXARGUMENT_DEPRECATE
		newCXTypeSignature.Meta = int(prgrm.cxArgumentSlot(cxArgument, true))
	}

	newCXTypeSignature.Offset = newCXTypeSignature.GetSize(prgrm, false)

	return newCXTypeSignature.Index
}

// RetypeCXTypeSignatureOfCXArg gives typeSig, which names cxArgument, the
// signature type of cxArgument once its type is known, as x := v learns it.
func RetypeCXTypeSignatureOfCXArg(prgrm *CXProgram, typeSig *CXTypeSignature, cxArgument *CXArgument) {
	if fillCXTypeSignatureOfCXArg(prgrm, typeSig, cxArgument) {
		typeSig.Offset = cxArgument.Offset
	}
}

// AllocCXTypeSignatureOfCXArg allocates the signature of cxArgument in the
// program's CXTypeSignatureMng and returns its slot. An argument with no
// signature type of its own is named by the signature, and a heap argument
// that has no slot yet is copied in first.
func AllocCXTypeSignatureOfCXArg(prgrm *CXProgram, cxArgument *CXArgument) CXTypeSignatureIndex {
	_, newCXTypeSignature := prgrm.AllocCXTypeSignature()
	if !fillCXTypeSignatureOfCXArg(prgrm, newCXTypeSignature, cxArgument) {
		// TYPE_CXARGUMENT_DEPRECATE, the CXArg slot is in Meta.
		newCXTypeSignature.Type = TYPE_CXARGUMENT_DEPRECATE
		newCXTypeSignature.Meta = int(prgrm.cxArgumentSlot(cxArgument, false))
		newCXTypeSignature.Offset = cxArgument.Offset
	}

	return newCXTypeSignature.Index
}

// GetCXTypeSignatureRepresentationOfCXArg builds the signature of cxArgument
// on the heap, for the callers that still copy it in with
// AddCXTypeSignatureInArray, the natives, serialization and tests.
func GetCXTypeSignatureRepresentationOfCXArg(prgrm *CXProgram, cxArgument *CXArgument) *CXTypeSignature {
	// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureAlloc(), phase 4 #331.
	// Now a value literal, site S0319.
	newCXTypeSignature := CXTypeSignature{}
	if !fillCXTypeSignatureOfCXArg(prgrm, &newCXTypeSignature, cxArgument) {
		// TYPE_CXARGUMENT_DEPRECATE
		// FieldIdx or the CXArg ID is in Meta field.
		newCXTypeSignature.Type = TYPE_CXARGUMENT_DEPRECATE
		newCXTypeSignature.Meta = int(prgrm.cxArgumentSlot(cxArgument, false))
		newCXTypeSignature.Offset = cxArgument.Offset
	}

	return &newCXTypeSignature
}

// AllocCXTypeSignatureOfCXArgBuilder is AllocCXTypeSignatureOfCXArg for a
// builder that nothing else holds. The signature takes it, and when the
// signature does not name it the builder is released.
func AllocCXTypeSignatureOfCXArgBuilder(prgrm *CXProgram, cxArgument *CXArgument) CXTypeSignatureIndex {
	typeSigIdx := AllocCXTypeSignatureOfCXArg(prgrm, cxArgument)
	prgrm.ReleaseCXArgUnlessNamed(cxArgument, typeSigIdx)
	return typeSigIdx
}

// AllocCXTypeSignatureOfCXArgDeprecate allocates a TYPE_CXARGUMENT_DEPRECATE
// signature that names cxArgument, whatever its type.
func AllocCXTypeSignatureOfCXArgDeprecate(prgrm *CXProgram, cxArgument *CXArgument) CXTypeSignatureIndex {
	_, typeSig := prgrm.AllocCXTypeSignature()
	typeSig.Name = cxArgument.Name
	typeSig.Package = cxArgument.Package
	typeSig.PassBy = cxArgument.PassBy
	typeSig.ArgDetails = cxArgument.ArgDetails
	typeSig.Type = TYPE_CXARGUMENT_DEPRECATE
	typeSig.Meta = int(prgrm.cxArgumentSlot(cxArgument, false))
	typeSig.Offset = cxArgument.Offset
	return typeSig.Index
}

// GetStructType returns the struct type of the value typeSignature names,
// or nil when it is not a struct or a pointer to one.
func (typeSignature *CXTypeSignature) GetStructType(prgrm *CXProgram) *CXStruct {
	switch typeSignature.Type {
	case TYPE_CXARGUMENT_DEPRECATE:
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1784.
		return prgrm.GetCXArg(CXArgumentIndex(typeSignature.Meta)).StructType
	case TYPE_STRUCT, TYPE_POINTER_STRUCT:
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0255.
		return prgrm.GetCXTypeSignatureStructFromArray(typeSignature.Meta).StructType
	}
	return nil
}

// GetOffset returns the offset of the value typeSignature names. A signature
// that names an argument has the offset of the argument.
func (typeSignature *CXTypeSignature) GetOffset(prgrm *CXProgram) types.Pointer {
	if typeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0260.
		return prgrm.GetCXArg(CXArgumentIndex(typeSignature.Meta)).Offset
	}
	return typeSignature.Offset
}

// CXTypeSignaturePassBy is the PassBy of sig, read from its argument when
// sig is a TYPE_CXARGUMENT_DEPRECATE.
func CXTypeSignaturePassBy(prgrm *CXProgram, sig *CXTypeSignature) int {
	if sig.Type == TYPE_CXARGUMENT_DEPRECATE {
		return prgrm.GetCXArgFromArray(CXArgumentIndex(sig.Meta)).PassBy
	}
	return sig.PassBy
}

// indexedValueSize returns the size of the value an indexed array or slice
// reads. r[1] of a [2][3]i32 is a [3]i32 and m[0] of a [][]i32 a slice.
func indexedValueSize(details *CXTypeSignature_Array) types.Pointer {
	if len(details.Indexes) >= len(details.Lengths) {
		return types.Code(details.Meta).Size()
	}
	return ValueSizeOfLengths(details.Lengths[len(details.Indexes):], types.Code(details.Meta).Size())
}
