package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// GCKind is what a GCType value is.
type GCKind uint8

const (
	// GCKindPointer is a pointer, followed to the value it points to.
	GCKindPointer GCKind = iota + 1
	// GCKindSlice is a slice, followed to each element.
	GCKindSlice
	// GCKindStr is a str, whose object holds no references.
	GCKindStr
	// GCKindArray is an array, walked element by element in place.
	GCKindArray
	// GCKindStruct is a struct, walked field by field in place.
	GCKindStruct
)

// GCType describes where a value holds pointers, slices and strs, for the
// mark and the pointer update of MarkAndCompact. A nil *GCType holds none.
type GCType struct {
	GCTypeKind GCKind
	// GCTypeElem is the pointee, slice element or array element, nil when
	// it holds no references.
	GCTypeElem *GCType
	// GCTypeElemSize is the size of a slice or array element.
	GCTypeElemSize types.Pointer
	// GCTypeLength is the length of an array.
	GCTypeLength types.Pointer
	// GCTypeFields are the fields of a struct that hold references.
	GCTypeFields []GCTypeField
}

// GCTypeField is a field of a struct that holds references.
type GCTypeField struct {
	GCTypeFieldOffset types.Pointer
	GCTypeFieldType   *GCType
}

// GCRoot is a value the collection starts from. A frame root has a frame
// offset, flagged when the local escaped, and an absolute root an address.
type GCRoot struct {
	GCRootOffset types.Pointer
	GCRootInner  types.Pointer
	GCRootType   *GCType
}

// GCTypeCache holds the GCTypes of a program, built at its first collection.
type GCTypeCache struct {
	// GCTypeCacheStructs is the GCType of each struct, nil for one that holds
	// no references.
	GCTypeCacheStructs map[*CXStruct]*GCType
	// GCTypeCacheFrames are the roots of the frame of each function.
	GCTypeCacheFrames map[*CXFunction]*GCFrameRoots
	// GCTypeCacheGlobals are the roots of the globals, built from
	// GCTypeCacheGlobalCount globals.
	GCTypeCacheGlobals     []GCRoot
	GCTypeCacheGlobalCount int
	// GCTypeCacheStr is the GCType of a str.
	GCTypeCacheStr *GCType
}

// GCFrameRoots are the roots of the frame of a function, built from
// GCFrameRootsCount entries of its ListOfSlots and ListOfPointers.
type GCFrameRoots struct {
	GCFrameRootsRoots []GCRoot
	GCFrameRootsCount int
}

// gcTypeCache returns the GCTypeCache of prgrm.
func gcTypeCache(prgrm *CXProgram) *GCTypeCache {
	if prgrm.Heap.Types == nil {
		prgrm.Heap.Types = &GCTypeCache{
			GCTypeCacheStructs: map[*CXStruct]*GCType{},
			GCTypeCacheFrames:  map[*CXFunction]*GCFrameRoots{},
			GCTypeCacheStr:     &GCType{GCTypeKind: GCKindStr},
		}
	}
	return prgrm.Heap.Types
}

// CXArgumentGCDeclaration returns the declared type of the variable arg
// names, without the dereferences and indexing of an expression.
func CXArgumentGCDeclaration(arg *CXArgument) ([]int, types.Code, *CXStruct, []types.Pointer) {
	specs := arg.DeclarationSpecifiers
	for i, spec := range specs {
		if spec == constants.DECL_DEREF || spec == constants.DECL_INDEXING {
			specs = append([]int(nil), specs[:i]...)
			for _, spec := range arg.DeclarationSpecifiers[i+1:] {
				if spec != constants.DECL_DEREF && spec != constants.DECL_INDEXING {
					specs = append(specs, spec)
				}
			}
			break
		}
	}
	typ := arg.Type
	if typ == types.POINTER {
		typ = arg.PointerTargetType
	}
	return specs, typ, arg.StructType, arg.Lengths
}

// SignatureGCDeclaration returns the declared type of the value sig names.
func SignatureGCDeclaration(prgrm *CXProgram, sig *CXTypeSignature) ([]int, types.Code, *CXStruct, []types.Pointer) {
	array := func(specs []int) ([]int, types.Code, []types.Pointer) {
		details := prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta)
		for range details.Lengths {
			specs = append(specs, constants.DECL_ARRAY)
		}
		return specs, types.Code(details.Meta), details.Lengths
	}
	switch sig.Type {
	case TYPE_CXARGUMENT_DEPRECATE:
		return CXArgumentGCDeclaration(prgrm.GetCXArg(CXArgumentIndex(sig.Meta)))
	case TYPE_ATOMIC:
		return []int{constants.DECL_BASIC}, types.Code(sig.Meta), nil, nil
	case TYPE_POINTER_ATOMIC:
		return []int{constants.DECL_BASIC, constants.DECL_POINTER}, types.Code(sig.Meta), nil, nil
	case TYPE_ARRAY_ATOMIC:
		specs, typ, lengths := array([]int{constants.DECL_BASIC})
		return specs, typ, nil, lengths
	case TYPE_POINTER_ARRAY_ATOMIC:
		specs, typ, lengths := array([]int{constants.DECL_BASIC})
		return append(specs, constants.DECL_POINTER), typ, nil, lengths
	case TYPE_SLICE_ATOMIC, TYPE_POINTER_SLICE_ATOMIC:
		details := prgrm.GetCXTypeSignatureArrayFromArray(sig.Meta)
		specs := []int{constants.DECL_BASIC, constants.DECL_SLICE}
		if sig.Type == TYPE_POINTER_SLICE_ATOMIC {
			specs = append(specs, constants.DECL_POINTER)
		}
		return specs, types.Code(details.Meta), nil, details.Lengths
	case TYPE_STRUCT:
		return []int{constants.DECL_STRUCT}, types.STRUCT, sig.GetStructType(prgrm), nil
	case TYPE_POINTER_STRUCT:
		return []int{constants.DECL_STRUCT, constants.DECL_POINTER}, types.STRUCT, sig.GetStructType(prgrm), nil
	}
	return nil, types.UNDEFINED, nil, nil
}

// GCTypeOf returns the GCType of a value of declaration specifiers specs,
// innermost first, base type typ or struct strct, and the lengths of its
// arrays and slices, outermost first. A slice may or may not have a length.
func GCTypeOf(prgrm *CXProgram, specs []int, typ types.Code, strct *CXStruct, lengths []types.Pointer) *GCType {
	cache := gcTypeCache(prgrm)
	if len(specs) == 0 {
		return cache.gcBaseType(prgrm, typ, strct)
	}
	inner := specs[:len(specs)-1]
	switch specs[len(specs)-1] {
	case constants.DECL_POINTER:
		return &GCType{GCTypeKind: GCKindPointer, GCTypeElem: GCTypeOf(prgrm, inner, typ, strct, lengths)}
	case constants.DECL_SLICE:
		if len(lengths) > gcArrayCount(inner) {
			lengths = lengths[1:]
		}
		slice := &GCType{GCTypeKind: GCKindSlice, GCTypeElem: GCTypeOf(prgrm, inner, typ, strct, lengths)}
		if slice.GCTypeElem != nil {
			slice.GCTypeElemSize = gcValueSize(prgrm, inner, typ, strct, lengths)
		}
		return slice
	case constants.DECL_ARRAY:
		if len(lengths) == 0 || lengths[0] == 0 {
			return nil
		}
		elem := GCTypeOf(prgrm, inner, typ, strct, lengths[1:])
		if elem == nil {
			return nil
		}
		return &GCType{GCTypeKind: GCKindArray, GCTypeElem: elem, GCTypeLength: lengths[0], GCTypeElemSize: gcValueSize(prgrm, inner, typ, strct, lengths[1:])}
	case constants.DECL_BASIC, constants.DECL_STRUCT:
		return cache.gcBaseType(prgrm, typ, strct)
	}
	return nil
}

// gcBaseType returns the GCType of a struct or basic value.
func (cache *GCTypeCache) gcBaseType(prgrm *CXProgram, typ types.Code, strct *CXStruct) *GCType {
	if strct == nil {
		if typ == types.STR {
			return cache.GCTypeCacheStr
		}
		return nil
	}
	if t, found := cache.GCTypeCacheStructs[strct]; found {
		return t
	}
	// A pointer or slice back to strct takes t before its fields are known.
	t := &GCType{GCTypeKind: GCKindStruct}
	cache.GCTypeCacheStructs[strct] = t
	for fieldIndex, fieldIdx := range strct.Fields {
		specs, typ, fieldStruct, lengths := SignatureGCDeclaration(prgrm, prgrm.GetCXTypeSignatureFromArray(fieldIdx))
		if field := GCTypeOf(prgrm, specs, typ, fieldStruct, lengths); field != nil {
			t.GCTypeFields = append(t.GCTypeFields, GCTypeField{GCTypeFieldOffset: StructFieldOffset(prgrm, strct, fieldIndex), GCTypeFieldType: field})
		}
	}
	if len(t.GCTypeFields) == 0 {
		cache.GCTypeCacheStructs[strct] = nil
		return nil
	}
	return t
}

// gcArrayCount returns how many arrays specs has.
func gcArrayCount(specs []int) int {
	var count int
	for _, spec := range specs {
		if spec == constants.DECL_ARRAY {
			count++
		}
	}
	return count
}

// gcValueSize returns the size of a value, as GCTypeOf reads its type.
func gcValueSize(prgrm *CXProgram, specs []int, typ types.Code, strct *CXStruct, lengths []types.Pointer) types.Pointer {
	if len(specs) == 0 {
		specs = []int{constants.DECL_BASIC}
	}
	switch specs[len(specs)-1] {
	case constants.DECL_POINTER, constants.DECL_SLICE:
		return types.POINTER_SIZE
	case constants.DECL_ARRAY:
		if len(lengths) == 0 {
			return 0
		}
		return lengths[0] * gcValueSize(prgrm, specs[:len(specs)-1], typ, strct, lengths[1:])
	}
	if strct != nil {
		return strct.GetStructSize(prgrm)
	}
	return typ.Size()
}

// gcFrameRoots returns the roots of the frame of fn.
func gcFrameRoots(prgrm *CXProgram, fn *CXFunction) []GCRoot {
	cache := gcTypeCache(prgrm)
	count := len(fn.ListOfSlots) + len(fn.ListOfPointers)
	if frame := cache.GCTypeCacheFrames[fn]; frame != nil && frame.GCFrameRootsCount == count {
		return frame.GCFrameRootsRoots
	}
	frame := &GCFrameRoots{GCFrameRootsCount: count}
	add := func(offset, inner types.Pointer, t *GCType) {
		if t != nil {
			frame.GCFrameRootsRoots = append(frame.GCFrameRootsRoots, GCRoot{GCRootOffset: offset, GCRootInner: inner, GCRootType: t})
		}
	}
	for _, slot := range fn.ListOfSlots {
		add(slot.CXFunctionSlotOffset, 0, GCTypeOf(prgrm, slot.CXFunctionSlotSpecifiers, slot.CXFunctionSlotType, slot.CXFunctionSlotStruct, slot.CXFunctionSlotLengths))
	}
	for _, ptr := range fn.ListOfPointers {
		switch {
		case ptr.IsPointer():
			specs, typ, strct, lengths := CXArgumentGCDeclaration(ptr)
			add(ptr.Offset, 0, GCTypeOf(prgrm, specs, typ, strct, lengths))
		case len(ptr.Fields) == 1:
			// A pointer field of a struct local, as s.p.
			fld := prgrm.GetCXArgFromArray(ptr.Fields[0])
			if fld.IsPointer() {
				specs, typ, strct, lengths := CXArgumentGCDeclaration(fld)
				add(ptr.Offset, fld.Offset, GCTypeOf(prgrm, specs, typ, strct, lengths))
			}
		}
	}
	cache.GCTypeCacheFrames[fn] = frame
	return frame.GCFrameRootsRoots
}

// gcGlobalRoots returns the roots of the globals of prgrm, at their
// addresses.
func gcGlobalRoots(prgrm *CXProgram) []GCRoot {
	cache := gcTypeCache(prgrm)
	var count int
	for _, pkgIdx := range prgrm.Packages {
		pkg, err := prgrm.GetPackageFromArray(pkgIdx)
		if err != nil {
			panic(err)
		}
		count += len(pkg.Globals.Fields)
	}
	if count == cache.GCTypeCacheGlobalCount && cache.GCTypeCacheGlobals != nil {
		return cache.GCTypeCacheGlobals
	}
	roots := []GCRoot{}
	for _, pkgIdx := range prgrm.Packages {
		pkg, err := prgrm.GetPackageFromArray(pkgIdx)
		if err != nil {
			panic(err)
		}
		for _, glblFldIdx := range pkg.Globals.Fields {
			sig := prgrm.GetCXTypeSignatureFromArray(glblFldIdx)
			offset := sig.Offset
			if sig.Type == TYPE_CXARGUMENT_DEPRECATE {
				offset = prgrm.GetCXArg(CXArgumentIndex(sig.Meta)).Offset
			}
			specs, typ, strct, lengths := SignatureGCDeclaration(prgrm, sig)
			if t := GCTypeOf(prgrm, specs, typ, strct, lengths); t != nil {
				roots = append(roots, GCRoot{GCRootOffset: offset, GCRootType: t})
			}
		}
	}
	cache.GCTypeCacheGlobals, cache.GCTypeCacheGlobalCount = roots, count
	return roots
}
