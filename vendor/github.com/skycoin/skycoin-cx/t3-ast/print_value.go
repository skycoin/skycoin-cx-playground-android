package ast

import (
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// Printing appends to the format buffer of the program and builds no Go
// string, so printf and sprintf of %v allocate nothing (#407, VMALLOC-015).

// printSpecifiersMax is the number of declaration specifiers appendArgValueAt
// keeps on its stack.
const printSpecifiersMax = 16

// printValueInvalid is printed for a value that would be read outside program
// memory, such as a local the call stack prints before its declaration ran.
const printValueInvalid = "<invalid address>"

// printMemoryFits reports whether size bytes at addr are inside memory.
func printMemoryFits(memory []byte, addr, size types.Pointer) bool {
	length := types.Cast_int_to_ptr(len(memory))
	return addr <= length && size <= length-addr
}

// appendPointerAt appends the pointer stored at addr as appendPointer does.
func appendPointerAt(dst []byte, memory []byte, addr types.Pointer) []byte {
	if !printMemoryFits(memory, addr, types.POINTER_SIZE) {
		return append(dst, printValueInvalid...)
	}
	return appendPointer(dst, types.Read_ptr(memory, addr))
}

// appendStrAt appends the str whose pointer is at addr. A nil str is empty,
// and a pointer to no whole object in memory prints printValueInvalid.
func appendStrAt(dst []byte, memory []byte, addr types.Pointer) []byte {
	obj := types.Read_ptr(memory, addr)
	if obj == 0 {
		return dst
	}
	if !printMemoryFits(memory, obj, types.OBJECT_HEADER_SIZE) {
		return append(dst, printValueInvalid...)
	}
	size := types.Read_obj_size(memory, obj)
	if size < types.OBJECT_HEADER_SIZE || !printMemoryFits(memory, obj, size) {
		return append(dst, printValueInvalid...)
	}
	return append(dst, memory[obj+types.OBJECT_HEADER_SIZE:obj+size]...)
}

// appendValueAt appends the value of type typ with dimensions dims at addr. A
// zero dimension is a slice, which holds a pointer to its object. The
// elements of a multi dimensional collection are separated by spaces.
func appendValueAt(dst []byte, prgrm *CXProgram, addr types.Pointer, typ types.Code, size types.Pointer, structType *CXStruct, dims []types.Pointer, spaced bool) []byte {
	if len(dims) == 0 {
		if typ == types.POINTER {
			return appendPointerAt(dst, prgrm.Memory, addr)
		}
		if structType != nil {
			return appendStructAt(dst, prgrm, addr, structType)
		}
		return appendAtomic(dst, prgrm.Memory, typ, addr)
	}

	stride := size
	for i := len(dims) - 1; i > 0; i-- {
		if dims[i] == 0 {
			stride = types.POINTER_SIZE
		} else {
			stride *= dims[i]
		}
	}

	n := dims[0]
	if n == 0 {
		if !printMemoryFits(prgrm.Memory, addr, types.POINTER_SIZE) {
			return append(dst, printValueInvalid...)
		}
		sliceOffset := types.Read_ptr(prgrm.Memory, addr)
		addr = sliceOffset + types.OBJECT_HEADER_SIZE + constants.SLICE_HEADER_SIZE
		if sliceOffset != 0 {
			if !printMemoryFits(prgrm.Memory, sliceOffset, types.OBJECT_HEADER_SIZE+constants.SLICE_HEADER_SIZE) {
				return append(dst, printValueInvalid...)
			}
			n = GetSliceLen(prgrm, sliceOffset)
		}
	}
	if n > 0 && stride > 0 && (!printMemoryFits(prgrm.Memory, addr, 0) || n > (types.Cast_int_to_ptr(len(prgrm.Memory))-addr)/stride) {
		return append(dst, printValueInvalid...)
	}
	dst = append(dst, '[')
	for c := types.Pointer(0); c < n; c++ {
		if c > 0 {
			if spaced {
				dst = append(dst, ' ')
			} else {
				dst = append(dst, ',', ' ')
			}
		}
		dst = appendValueAt(dst, prgrm, addr+c*stride, typ, size, structType, dims[1:], spaced)
	}
	return append(dst, ']')
}

// appendStructAt appends the fields of the struct instance at addr as
// {name: value, ...}.
func appendStructAt(dst []byte, prgrm *CXProgram, addr types.Pointer, strct *CXStruct) []byte {
	dst = append(dst, '{')
	off := types.Pointer(0)
	for c, fldIdx := range strct.Fields {
		// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
		// Now an arena getter, site S0487.
		fld := prgrm.GetCXTypeSignatureFromArray(fldIdx)
		if c > 0 {
			dst = append(dst, ',', ' ')
		}
		dst = append(dst, prgrm.CXProgramNameBytes(fld.Name)...)
		dst = append(dst, ':', ' ')
		dst = appendTypeSignatureAt(dst, prgrm, addr+off, fld)
		off += fld.GetSize(prgrm, false)
	}
	return append(dst, '}')
}

// appendTypeSignatureAt appends the value of the struct field typeSig at addr.
func appendTypeSignatureAt(dst []byte, prgrm *CXProgram, addr types.Pointer, typeSig *CXTypeSignature) []byte {
	switch typeSig.Type {
	case TYPE_ATOMIC:
		return appendAtomic(dst, prgrm.Memory, types.Code(typeSig.Meta), addr)
	case TYPE_ARRAY_ATOMIC, TYPE_SLICE_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0488.
		arr := prgrm.GetCXTypeSignatureArrayFromArray(typeSig.Meta)
		eltType := types.Code(arr.Meta)
		return appendValueAt(dst, prgrm, addr, eltType, eltType.Size(), nil, arr.Lengths, len(arr.Lengths) > 1)
	case TYPE_STRUCT:
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0490.
		return appendStructAt(dst, prgrm, addr, prgrm.GetCXTypeSignatureStructFromArray(typeSig.Meta).StructType)
	case TYPE_CXARGUMENT_DEPRECATE:
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0491.
		return appendArgValueAt(dst, prgrm, addr, prgrm.GetCXArgFromArray(CXArgumentIndex(typeSig.Meta)))
	}
	// Pointers print their address.
	return appendPointerAt(dst, prgrm.Memory, addr)
}

// appendAddress appends p in hexadecimal with 0x, as fmt prints %#x.
func appendAddress(dst []byte, p types.Pointer) []byte {
	return strconv.AppendUint(append(dst, '0', 'x'), uint64(p), 16)
}

// appendPointer appends the pointer p as Go prints it, its address, or <nil>
// when it is nil.
func appendPointer(dst []byte, p types.Pointer) []byte {
	if p == 0 {
		return append(dst, '<', 'n', 'i', 'l', '>')
	}
	return appendAddress(dst, p)
}

// appendStructPointerAt appends the pointer to a struct strct stored at addr
// as Go prints a pointer to a struct that it is given, &{...}, or <nil>.
func appendStructPointerAt(dst []byte, prgrm *CXProgram, addr types.Pointer, strct *CXStruct) []byte {
	if !printMemoryFits(prgrm.Memory, addr, types.POINTER_SIZE) {
		return append(dst, printValueInvalid...)
	}
	p := types.Read_ptr(prgrm.Memory, addr)
	if p == 0 || strct == nil {
		return appendPointer(dst, p)
	}
	if p >= prgrm.Heap.StartsAt {
		p += types.OBJECT_HEADER_SIZE
	}
	if !printMemoryFits(prgrm.Memory, p, strct.GetStructSize(prgrm)) {
		return append(dst, printValueInvalid...)
	}
	return appendStructAt(append(dst, '&'), prgrm, p, strct)
}

// printSpecifiers appends to specs the specifiers of the value elt reads,
// without those its dereferences and indexes remove.
func printSpecifiers(elt *CXArgument, specs []int) []int {
	for _, spec := range elt.DeclarationSpecifiers {
		switch spec {
		case constants.DECL_DEREF, constants.DECL_INDEXING:
			if len(specs) > 0 {
				specs = specs[:len(specs)-1]
			}
		default:
			specs = append(specs, spec)
		}
	}
	return specs
}

// appendAtomic appends the value of the atomic type typ at offset in memory
// as fmt prints it with %v. Other types append nothing.
func appendAtomic(dst []byte, memory []byte, typ types.Code, offset types.Pointer) []byte {
	switch typ {
	case types.BOOL, types.STR, types.I8, types.I16, types.I32, types.I64, types.UI8, types.UI16, types.UI32, types.UI64, types.F32, types.F64:
		if !printMemoryFits(memory, offset, typ.Size()) {
			return append(dst, printValueInvalid...)
		}
	}
	switch typ {
	case types.BOOL:
		return strconv.AppendBool(dst, types.Read_bool(memory, offset))
	case types.STR:
		return appendStrAt(dst, memory, offset)
	case types.I8:
		return strconv.AppendInt(dst, int64(types.Read_i8(memory, offset)), 10)
	case types.I16:
		return strconv.AppendInt(dst, int64(types.Read_i16(memory, offset)), 10)
	case types.I32:
		return strconv.AppendInt(dst, int64(types.Read_i32(memory, offset)), 10)
	case types.I64:
		return strconv.AppendInt(dst, types.Read_i64(memory, offset), 10)
	case types.UI8:
		return strconv.AppendUint(dst, uint64(types.Read_ui8(memory, offset)), 10)
	case types.UI16:
		return strconv.AppendUint(dst, uint64(types.Read_ui16(memory, offset)), 10)
	case types.UI32:
		return strconv.AppendUint(dst, uint64(types.Read_ui32(memory, offset)), 10)
	case types.UI64:
		return strconv.AppendUint(dst, types.Read_ui64(memory, offset), 10)
	case types.F32:
		return strconv.AppendFloat(dst, float64(types.Read_f32(memory, offset)), 'g', -1, 32)
	case types.F64:
		return strconv.AppendFloat(dst, types.Read_f64(memory, offset), 'g', -1, 64)
	}
	return dst
}

// appendArgValueAt appends the value that elt, the last element of an
// argument's field path, names at addr after its indexes and dereferences.
func appendArgValueAt(dst []byte, prgrm *CXProgram, addr types.Pointer, elt *CXArgument) []byte {
	typ := elt.Type
	if elt.IsPointer() {
		for _, op := range elt.DereferenceOperations {
			if op == constants.DEREF_POINTER {
				typ = elt.PointerTargetType
				break
			}
		}
	}
	dims := elt.Lengths
	if len(elt.Indexes) <= len(dims) {
		dims = dims[len(elt.Indexes):]
	}
	var specsStack [printSpecifiersMax]int
	specs := printSpecifiers(elt, specsStack[:0])
	if direct, behind := elt.DirectDimensions(); behind && len(elt.Indexes) <= direct {
		// The elements of [N]*[M]T are pointers.
		dims = elt.Lengths[len(elt.Indexes):direct]
		return appendValueAt(dst, prgrm, addr, types.POINTER, types.POINTER_SIZE, nil, dims, len(dims) > 1)
	}
	if len(specs) > 1 && specs[1] == constants.DECL_POINTER && (len(dims) > 0 || len(specs) == 2) {
		if len(dims) == 0 && elt.StructType != nil {
			return appendStructPointerAt(dst, prgrm, addr, elt.StructType)
		}
		// A pointer, or an array or slice of pointers, prints addresses.
		return appendValueAt(dst, prgrm, addr, types.POINTER, types.POINTER_SIZE, nil, dims, len(dims) > 1)
	}
	if len(dims) == 0 && len(specs) > 2 && specs[len(specs)-1] == constants.DECL_POINTER {
		return appendPointerAt(dst, prgrm.Memory, addr)
	}
	return appendValueAt(dst, prgrm, addr, typ, elt.Size, elt.StructType, dims, len(dims) > 1)
}

// AppendPrintableValue appends the value of argTypeSig in the frame at fp to
// dst as printf prints it with %v.
func AppendPrintableValue(dst []byte, prgrm *CXProgram, fp types.Pointer, argTypeSig *CXTypeSignature) []byte {
	switch argTypeSig.Type {
	case TYPE_CXARGUMENT_DEPRECATE:
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S0493.
		arg := prgrm.GetCXArgFromArray(CXArgumentIndex(argTypeSig.Meta))
		elt := arg.GetAssignmentElement(prgrm)
		addr := GetFinalOffset(prgrm, fp, nil, argTypeSig)
		if arg.PassBy == constants.PASSBY_REFERENCE {
			// &v prints the address of v, or &{...} for a struct.
			if elt.StructType != nil && len(elt.Lengths) <= len(elt.Indexes) {
				return appendStructAt(append(dst, '&'), prgrm, addr, elt.StructType)
			}
			return appendAddress(dst, addr)
		}
		return appendArgValueAt(dst, prgrm, addr, elt)
	case TYPE_ATOMIC:
		if argTypeSig.PassBy == constants.PASSBY_REFERENCE {
			return appendAddress(dst, GetFinalOffset(prgrm, fp, nil, argTypeSig))
		}
		return appendAtomic(dst, prgrm.Memory, types.Code(argTypeSig.Meta), GetFinalOffset(prgrm, fp, nil, argTypeSig))
	case TYPE_POINTER_ATOMIC:
		if argTypeSig.IsDeref {
			return appendAtomic(dst, prgrm.Memory, types.Code(argTypeSig.Meta), GetFinalOffset(prgrm, fp, nil, argTypeSig))
		}
		return appendPointerAt(dst, prgrm.Memory, GetFinalOffset(prgrm, fp, nil, argTypeSig))
	case TYPE_ARRAY_ATOMIC, TYPE_POINTER_ARRAY_ATOMIC, TYPE_SLICE_ATOMIC, TYPE_POINTER_SLICE_ATOMIC:
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S0494.
		arr := prgrm.GetCXTypeSignatureArrayFromArray(argTypeSig.Meta)
		eltType := types.Code(arr.Meta)
		dims := arr.Lengths
		if len(arr.Indexes) <= len(dims) {
			dims = dims[len(arr.Indexes):]
		}
		addr := GetFinalOffset(prgrm, fp, nil, argTypeSig)
		pointer := argTypeSig.Type == TYPE_POINTER_ARRAY_ATOMIC || argTypeSig.Type == TYPE_POINTER_SLICE_ATOMIC
		if pointer && !argTypeSig.IsDeref {
			return appendPointerAt(dst, prgrm.Memory, addr)
		}
		return appendValueAt(dst, prgrm, addr, eltType, eltType.Size(), nil, dims, len(dims) > 1)
	case TYPE_STRUCT, TYPE_POINTER_STRUCT:
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S0496.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(argTypeSig.Meta)
		addr := GetFinalOffset(prgrm, fp, nil, argTypeSig)
		if argTypeSig.Type == TYPE_POINTER_STRUCT && !argTypeSig.IsDeref && len(structDetails.Fields) == 0 {
			return appendStructPointerAt(dst, prgrm, addr, structDetails.StructType)
		}
		if argTypeSig.Type == TYPE_STRUCT && argTypeSig.PassBy == constants.PASSBY_REFERENCE && len(structDetails.Fields) == 0 {
			return appendStructAt(append(dst, '&'), prgrm, addr, structDetails.StructType)
		}
		if n := len(structDetails.Fields); n > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S0497.
			return appendArgValueAt(dst, prgrm, addr, prgrm.GetCXArgFromArray(structDetails.Fields[n-1]))
		}
		return appendStructAt(dst, prgrm, addr, structDetails.StructType)
	}
	panic("type is not known")
}
