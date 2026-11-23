package ast

import (
	"fmt"

	types "github.com/skycoin/skycoin-cx/t0-types"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

type CXStructIndex int

// ----------------------------------------------------------------
//                             `CXStruct` Getters

// GetField returns the signature of the field name of strct. Its
// GetStructType and GetOffset give what a caller needs of the field.
func (strct *CXStruct) GetField(prgrm *CXProgram, name stringpool.StringGid) (*CXTypeSignature, error) {
	for _, typeSignatureIdx := range strct.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0254.
		typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
		if typeSignature.Name == name {
			return typeSignature, nil
		}
	}

	return nil, fmt.Errorf("field '%s' not found in struct '%s'", prgrm.CXProgramNameText(name), prgrm.CXProgramNameText(strct.Name))
}

// ----------------------------------------------------------------
//                     `CXStruct` Member handling

// MakeStruct ...
// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
// Now a constructor, site S0261.
func MakeStruct(name stringpool.StringGid) *CXStruct {
	// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
	// Now a heap literal, site S0262.
	return &CXStruct{
		Name: name,
	}
}

// AddField adds the field cxArgument to strct. It takes a managed cxArgument,
// which it releases when the signature does not name it.
func (strct *CXStruct) AddField(prgrm *CXProgram, cxArgument *CXArgument) *CXStruct {
	// Check if field already exist
	for _, typeSignatureIdx := range strct.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0263.
		typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
		if typeSignature.Name == cxArgument.Name {
			prgrm.CXProgramCompileErrorAddDebug(CompileErrorCodeStructFieldDuplicate, cxArgument.ArgDetails, CompileErrorOperands{int64(cxArgument.Name)})
			prgrm.ReleaseCXArgBuilder(cxArgument)
			return strct
		}
	}

	typeSignatureIdx := AllocCXTypeSignatureOfCXArgForStructs(prgrm, cxArgument)
	prgrm.ReleaseCXArgUnlessNamed(cxArgument, typeSignatureIdx)
	strct.Fields = append(strct.Fields, typeSignatureIdx)

	// TODO: Found out the effect and completely remove this.
	// Dont remove this yet as removing this gives another error on cxfx
	// even though all cx tests passed already.
	// numFlds := len(strct.Fields)
	// if numFlds > 2 {
	// 	// Pre-compiling the offset of the field.
	// 	lastTypeSignature := strct.Fields[numFlds-2]
	// 	currentTypeSignature := strct.Fields[numFlds-1]
	// 	if lastTypeSignature.Type == TYPE_CXARGUMENT_DEPRECATE && currentTypeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
	// 		lastFldIdx := lastTypeSignature.Meta
	// 		currFldIdx := currentTypeSignature.Meta
	// 		prgrm.GetCXArg(CXArgumentIndex(currFldIdx)).Offset = prgrm.GetCXArg(CXArgumentIndex(lastFldIdx)).Offset + prgrm.GetCXArg(CXArgumentIndex(lastFldIdx)).TotalSize
	// 	} else if lastTypeSignature.Type == TYPE_CXARGUMENT_DEPRECATE && currentTypeSignature.Type != TYPE_CXARGUMENT_DEPRECATE {
	// 		lastFldIdx := lastTypeSignature.Meta
	// 		currentTypeSignature.Offset = prgrm.GetCXArg(CXArgumentIndex(lastFldIdx)).Offset + prgrm.GetCXArg(CXArgumentIndex(lastFldIdx)).TotalSize
	// 	} else if lastTypeSignature.Type != TYPE_CXARGUMENT_DEPRECATE && currentTypeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
	// 		currFldIdx := currentTypeSignature.Meta
	// 		prgrm.GetCXArg(CXArgumentIndex(currFldIdx)).Offset = lastTypeSignature.Offset + lastTypeSignature.GetSize(prgrm, false)
	// 	} else if lastTypeSignature.Type != TYPE_CXARGUMENT_DEPRECATE && currentTypeSignature.Type != TYPE_CXARGUMENT_DEPRECATE {
	// 		currentTypeSignature.Offset = lastTypeSignature.Offset + lastTypeSignature.GetSize(prgrm, false)
	// 	}
	// }

	return strct
}

// AddField ...
func (strct *CXStruct) AddField_TypeSignature(prgrm *CXProgram, fieldIdx CXTypeSignatureIndex) *CXStruct {
	// Check if field already exist
	for _, typeSignatureIdx := range strct.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0265.
		typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0266.
		field := prgrm.GetCXTypeSignatureFromArray(fieldIdx)
		if typeSignature.Name == field.Name {
			prgrm.CXProgramCompileErrorAddDebug(CompileErrorCodeStructFieldDuplicate, field.ArgDetails, CompileErrorOperands{int64(field.Name)})
			return strct
		}
	}

	strct.Fields = append(strct.Fields, fieldIdx)

	return strct
}

func (strct *CXStruct) AddField_CXAtomicOps(prgrm *CXProgram, fieldIdx CXTypeSignatureIndex) *CXStruct {
	strct.Fields = append(strct.Fields, fieldIdx)

	return strct
}

func (strct *CXStruct) AddField_Globals_CXAtomicOps(prgrm *CXProgram, cxArgIdx CXArgumentIndex) *CXStruct {
	// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
	// Now an arena getter, site S0267.
	cxArgument := prgrm.GetCXArgFromArray(cxArgIdx)

	// Check if field already exist
	for _, typeSignatureIdx := range strct.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0268.
		typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
		if typeSignature.Name == cxArgument.Name {
			prgrm.CXProgramCompileErrorAddDebug(CompileErrorCodeStructFieldDuplicate, cxArgument.ArgDetails, CompileErrorOperands{int64(cxArgument.Name)})
			return strct
		}
	}

	newCXTypeSignatureIdx := AllocCXTypeSignatureOfCXArg(prgrm, cxArgument)
	prgrm.ReleaseCXArgUnlessNamed(cxArgument, newCXTypeSignatureIdx)
	strct.Fields = append(strct.Fields, newCXTypeSignatureIdx)

	return strct
}

// RemoveField ...
// func (strct *CXStruct) RemoveField(prgrm *CXProgram, fldName string) {
// 	if len(strct.Fields) > 0 {
// 		lenFlds := len(strct.Fields)
// 		for i, fldIdx := range strct.Fields {
// 			if prgrm.GetCXArg(CXArgumentIndex(fldIdx)).Name == fldName {
// 				if i == lenFlds-1 {
// 					strct.Fields = strct.Fields[:len(strct.Fields)-1]
// 				} else {
// 					strct.Fields = append(strct.Fields[:i], strct.Fields[i+1:]...)
// 				}
// 				break
// 			}
// 		}
// 	}
// }

func (strct *CXStruct) GetStructSize(prgrm *CXProgram) types.Pointer {
	var structSize types.Pointer
	for _, typeSignatureIdx := range strct.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0270.
		typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)
		structSize += typeSignature.GetSize(prgrm, false)
	}

	return structSize
}

// StructFieldOffset returns the offset of field index of strct. The
// signature of a field holds its size in Offset, so the offset is a sum.
func StructFieldOffset(prgrm *CXProgram, strct *CXStruct, index int) types.Pointer {
	var offset types.Pointer
	for _, fieldIdx := range strct.Fields[:index] {
		offset += prgrm.GetCXTypeSignatureFromArray(fieldIdx).Offset
	}
	return offset
}
