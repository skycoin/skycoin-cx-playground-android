package opcodes

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// TODO: Rework
func opSliceLen(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var inp0 *ast.CXArgument
	var sliceLen types.Pointer
	if inputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1310.
		inp0 = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[0].TypeSignature.Meta))

		elt := inp0.GetAssignmentElement(prgrm)

		if elt.IsSlice() || elt.Type == types.AFF { //TODO: FIX
			sliceOffset := types.Read_ptr(prgrm.Memory, inputs[0].Offset)
			if sliceOffset > 0 && sliceOffset.IsValid() {
				sliceLen = ast.GetSliceLen(prgrm, sliceOffset)
			}
			// TODO: Had to add elt.Lengths to avoid doing this for arrays, but not entirely sure why
		} else if (elt.PointerTargetType == types.STR || elt.Type == types.STR) && elt.Lengths == nil {
			sliceLen = types.Read_str_size(prgrm.Memory, inputs[0].Offset)
		} else {
			sliceLen = elt.Lengths[len(elt.Indexes)]
		}
	} else if inputs[0].TypeSignature.Type == ast.TYPE_ARRAY_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1311.
		arrDetails := prgrm.GetCXTypeSignatureArrayFromArray(inputs[0].TypeSignature.Meta)
		sliceLen = arrDetails.Lengths[len(arrDetails.Indexes)]
	} else if inputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC || inputs[0].TypeSignature.Type == ast.TYPE_POINTER_SLICE_ATOMIC && inputs[0].TypeSignature.IsDeref {
		sliceOffset := types.Read_ptr(prgrm.Memory, inputs[0].Offset)
		if sliceOffset > 0 && sliceOffset.IsValid() {
			sliceLen = ast.GetSliceLen(prgrm, sliceOffset)
		}
	} else if inputs[0].TypeSignature.Type == ast.TYPE_STRUCT || inputs[0].TypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S1312.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(inputs[0].TypeSignature.Meta)

		fldLen := len(structDetails.Fields)
		if fldLen > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1313.
			fld := prgrm.GetCXArgFromArray(structDetails.Fields[fldLen-1])
			fld = fld.GetAssignmentElement(prgrm)

			if fld.IsSlice() || fld.Type == types.AFF { //TODO: FIX
				sliceOffset := types.Read_ptr(prgrm.Memory, inputs[0].Offset)
				if sliceOffset > 0 && sliceOffset.IsValid() {
					sliceLen = ast.GetSliceLen(prgrm, sliceOffset)
				}
				// TODO: Had to add elt.Lengths to avoid doing this for arrays, but not entirely sure why
			} else if (fld.PointerTargetType == types.STR || fld.Type == types.STR) && fld.Lengths == nil {
				sliceLen = types.Read_str_size(prgrm.Memory, inputs[0].Offset)
			} else {
				sliceLen = fld.Lengths[len(fld.Indexes)]
			}
		} else {
			panic("struct has no fields")
		}
	} else {
		panic("type is not known\n\n")
	}

	outputs[0].Set_i32(prgrm, types.Cast_ptr_to_i32(sliceLen)) // TODO:PTR remove hardcode i32, should use ptr alias.
}

// TODO: Rework
func opSliceAppend(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var inp0, out0 *ast.CXArgument
	var inp0Type, inp1Type, out0Type types.Code
	var input0IsSlice, outputIsSlice bool
	if inputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1314.
		inp0 = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[0].TypeSignature.Meta))
		inp0Type = inp0.Type
		if inp0.Type == types.POINTER {
			inp0Type = inp0.PointerTargetType
		}

		eltInp0 := inp0.GetAssignmentElement(prgrm)
		input0IsSlice = eltInp0.IsSlice()
	} else if inputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC || inputs[0].TypeSignature.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1315.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(inputs[0].TypeSignature.Meta)
		inp0Type = types.Code(sliceDetails.Meta)

		input0IsSlice = true
	} else if inputs[0].TypeSignature.Type == ast.TYPE_STRUCT || inputs[0].TypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		inp0Type = types.STRUCT

		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S1316.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(inputs[0].TypeSignature.Meta)
		fldLen := len(structDetails.Fields)
		if fldLen > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1317.
			fld := prgrm.GetCXArgFromArray(structDetails.Fields[fldLen-1])
			fld = fld.GetAssignmentElement(prgrm)
			input0IsSlice = fld.IsSlice()

			inp0Type = fld.Type
			if fld.Type == types.POINTER {
				inp0Type = fld.PointerTargetType
			}
		}
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	if inputs[1].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1318.
		inp1 := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[1].TypeSignature.Meta))
		inp1Type = inp1.Type
		if inp1.Type == types.POINTER {
			inp1Type = inp1.PointerTargetType
		}
	} else {
		inp1Type = inputs[1].TypeSignature.GetType(prgrm)
	}

	if outputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1319.
		out0 = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outputs[0].TypeSignature.Meta))

		out0 := out0.GetAssignmentElement(prgrm)

		out0Type = out0.Type
		if out0.Type == types.POINTER {
			out0Type = out0.PointerTargetType
		}

		outputIsSlice = out0.IsSlice()

	} else if outputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC || outputs[0].TypeSignature.Type == ast.TYPE_POINTER_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1320.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(outputs[0].TypeSignature.Meta)
		out0Type = types.Code(sliceDetails.Meta)
		outputIsSlice = true
	} else if outputs[0].TypeSignature.Type == ast.TYPE_STRUCT || outputs[0].TypeSignature.Type == ast.TYPE_POINTER_STRUCT {
		out0Type = types.STRUCT

		// OBJMODEL: state.CXTypeSignature_StructMng.CXTypeSignature_StructGet(gid), phase 5 #322.
		// Now an arena getter, site S1321.
		structDetails := prgrm.GetCXTypeSignatureStructFromArray(outputs[0].TypeSignature.Meta)
		fldLen := len(structDetails.Fields)
		if fldLen > 0 {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1322.
			fld := prgrm.GetCXArgFromArray(structDetails.Fields[fldLen-1])
			fld = fld.GetAssignmentElement(prgrm)
			outputIsSlice = fld.IsSlice()

			out0Type = fld.Type
			if fld.Type == types.POINTER {
				out0Type = fld.PointerTargetType
			}
		}
	} else {
		panic("type is not known")
	}

	sliceInputs := inputs[1:]
	sliceInputsLen := types.Cast_int_to_ptr(len(sliceInputs))

	if inp0Type != inp1Type || inp0Type != out0Type || !input0IsSlice || !outputIsSlice {
		panic(types.CX_RUNTIME_INVALID_ARGUMENT)
	}

	var inputSliceLen types.Pointer
	inputSliceOffset := types.Read_ptr(prgrm.Memory, inputs[0].Offset)
	if inputSliceOffset != 0 && inputSliceOffset.IsValid() {
		inputSliceLen = ast.GetSliceLen(prgrm, inputSliceOffset)
	}

	// Preparing slice in case more memory is needed for the new element.
	eltSize := ast.GetDerefSizeSlice(prgrm, inputs[0].TypeSignature)
	outputSliceOffset := ast.SliceAppendResize(prgrm, inputs[0].FramePointer, outputs[0].TypeSignature, inputs[0].TypeSignature, eltSize, sliceInputsLen)

	// We need to update the address of the output and input, as the final offsets
	// could be on the heap and they could have been moved by the GC.

	for i, input := range sliceInputs {
		var inp *ast.CXArgument
		var inpType types.Code
		if input.TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
			// Now an arena getter, site S1323.
			inp = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(input.TypeSignature.Meta))
			inpType = inp.Type
			if inp.Type == types.POINTER {
				inpType = inp.PointerTargetType
			}
		} else {
			inpType = input.TypeSignature.GetType(prgrm)
		}

		if inp0Type != inpType {
			panic(types.CX_RUNTIME_INVALID_ARGUMENT)
		}
		if ast.CXTypeSignaturePassBy(prgrm, input.TypeSignature) == constants.PASSBY_REFERENCE {
			// append(ps, &x) appends the address of x.
			var obj [types.POINTER_SIZE]byte
			types.Write_ptr(obj[:], 0, ast.PointerOfAddress(prgrm, input.Offset))
			ast.SliceAppendWrite(prgrm, outputSliceOffset, obj[:], inputSliceLen+types.Cast_int_to_ptr(i))
		} else if (inpType == types.STR || inpType == types.AFF) && eltSize == types.POINTER_SIZE {
			// An array of str, as [][2]str appends, is copied whole below.
			var obj [types.POINTER_SIZE]byte
			types.Write_ptr(obj[:], 0, types.Read_ptr(prgrm.Memory, input.Offset))
			ast.SliceAppendWrite(prgrm, outputSliceOffset, obj[:], inputSliceLen+types.Cast_int_to_ptr(i))
		} else {
			obj := types.GetSlice_byte(prgrm.Memory, input.Offset, eltSize)
			ast.SliceAppendWrite(prgrm, outputSliceOffset, obj, inputSliceLen+types.Cast_int_to_ptr(i))
		}
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
}

// TODO: Rework
func opSliceResize(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var inpSize types.Pointer
	var inpType, outType types.Code
	var inpIsSlice, outIsSlice bool
	if inputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1324.
		inp0 := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[0].TypeSignature.Meta))
		eltInp0 := inp0.GetAssignmentElement(prgrm)
		inpSize = eltInp0.Size

		inpType = inp0.Type
		inpIsSlice = eltInp0.IsSlice()
	} else if inputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1325.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(inputs[0].TypeSignature.Meta)
		inpSize = types.Code(sliceDetails.Meta).Size()

		inpType = types.Code(sliceDetails.Meta)
		inpIsSlice = true
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	if outputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1326.
		out0 := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outputs[0].TypeSignature.Meta))
		outType = out0.Type
		outIsSlice = out0.GetAssignmentElement(prgrm).IsSlice()
	} else if outputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1327.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(outputs[0].TypeSignature.Meta)

		outType = types.Code(sliceDetails.Meta)
		outIsSlice = true
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	fp := inputs[0].FramePointer

	if inpType != outType || !inpIsSlice || !outIsSlice {
		panic(types.CX_RUNTIME_INVALID_ARGUMENT)
	}

	outputSliceOffset := ast.SliceResize(prgrm, fp, outputs[0].TypeSignature, inputs[0].TypeSignature, types.Cast_i32_to_ptr(inputs[1].Get_i32(prgrm)), inpSize)

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
}

// TODO: Rework
func opSliceInsertElement(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var inp0Type, inp2Type, out0Type types.Code
	var inpIsSlice, outIsSlice bool
	if inputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1328.
		inp0 := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[0].TypeSignature.Meta))

		inp0Type = inp0.Type
		if inp0.Type == types.POINTER {
			inp0Type = inp0.PointerTargetType
		}

		inpIsSlice = inp0.IsSlice()
	} else if inputs[0].TypeSignature.Type == ast.TYPE_ATOMIC || inputs[0].TypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
		inp0Type = types.Code(inputs[0].TypeSignature.Meta)

		inpIsSlice = false
	} else if inputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1329.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(inputs[0].TypeSignature.Meta)

		inp0Type = types.Code(sliceDetails.Meta)
		inpIsSlice = true
	} else {
		panic("type is not known")
	}

	if inputs[2].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1330.
		inp2 := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[2].TypeSignature.Meta))

		inp2Type = inp2.Type
		if inp2.Type == types.POINTER {
			inp2Type = inp2.PointerTargetType
		}
	} else {
		inp2Type = inputs[2].TypeSignature.GetType(prgrm)
	}

	if outputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1331.
		out0 := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outputs[0].TypeSignature.Meta))

		out0Type = out0.Type
		if out0.Type == types.POINTER {
			out0Type = out0.PointerTargetType
		}

		outIsSlice = out0.IsSlice()
	} else if outputs[0].TypeSignature.Type == ast.TYPE_ATOMIC || outputs[0].TypeSignature.Type == ast.TYPE_POINTER_ATOMIC {
		out0Type = types.Code(outputs[0].TypeSignature.Meta)

		outIsSlice = false
	} else if outputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1332.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(outputs[0].TypeSignature.Meta)

		out0Type = types.Code(sliceDetails.Meta)
		outIsSlice = true
	} else {
		panic("type is not known")
	}

	fp := inputs[0].FramePointer

	if inp0Type != inp2Type || inp0Type != out0Type || !inpIsSlice || !outIsSlice {
		panic(types.CX_RUNTIME_INVALID_ARGUMENT)
	}

	index := types.Cast_i32_to_ptr(inputs[1].Get_i32(prgrm))

	var outputSliceOffset types.Pointer
	if inp2Type == types.STR || inp2Type == types.AFF {
		var obj [types.POINTER_SIZE]byte
		types.Write_ptr(obj[:], 0, types.Read_ptr(prgrm.Memory, inputs[2].Offset))
		outputSliceOffset = ast.SliceInsert(prgrm, fp, outputs[0].TypeSignature, inputs[0].TypeSignature, index, obj[:])
	} else {
		obj := inputs[2].Get_bytes(prgrm)
		outputSliceOffset = ast.SliceInsert(prgrm, fp, outputs[0].TypeSignature, inputs[0].TypeSignature, index, obj)
	}

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
}

// TODO: Rework
func opSliceRemoveElement(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var inpType, outType types.Code
	var inpIsSlice, outIsSlice bool
	var inpSize types.Pointer

	if inputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1333.
		inp0 := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[0].TypeSignature.Meta))

		inpType = inp0.Type
		inpIsSlice = inp0.GetAssignmentElement(prgrm).IsSlice()
		inpSize = inp0.GetAssignmentElement(prgrm).Size
	} else if inputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1334.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(inputs[0].TypeSignature.Meta)

		inpType = types.Code(sliceDetails.Meta)
		inpIsSlice = true
		inpSize = inpType.Size()
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	if outputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1335.
		out0 := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(outputs[0].TypeSignature.Meta))

		outType = out0.Type
		outIsSlice = out0.GetAssignmentElement(prgrm).IsSlice()
	} else if outputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1336.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(outputs[0].TypeSignature.Meta)

		outType = types.Code(sliceDetails.Meta)
		outIsSlice = true
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	fp := inputs[0].FramePointer

	if inpType != outType || !inpIsSlice || !outIsSlice {
		panic(types.CX_RUNTIME_INVALID_ARGUMENT)
	}

	outputSliceOffset := ast.SliceRemove(prgrm, fp, outputs[0].TypeSignature, inputs[0].TypeSignature, types.Cast_i32_to_ptr(inputs[1].Get_i32(prgrm)), inpSize)

	outputs[0].Set_ptr(prgrm, outputSliceOffset)
}

func opSliceCopy(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var dstType types.Code
	var dstSize types.Pointer
	var dstIsSlice bool
	if inputs[0].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1337.
		dstInput := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[0].TypeSignature.Meta))
		dstElem := dstInput.GetAssignmentElement(prgrm)

		dstType = dstInput.Type
		dstSize = dstElem.Size
		dstIsSlice = dstElem.IsSlice()
	} else if inputs[0].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1338.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(inputs[0].TypeSignature.Meta)

		dstType = types.Code(sliceDetails.Meta)
		dstSize = types.Code(sliceDetails.Meta).Size()
		dstIsSlice = true
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	var srcType types.Code
	var srcSize types.Pointer
	var srcIsSlice bool
	if inputs[1].TypeSignature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
		// Now an arena getter, site S1339.
		srcInput := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(inputs[1].TypeSignature.Meta))
		srcElem := srcInput.GetAssignmentElement(prgrm)

		srcType = srcInput.Type
		srcSize = srcElem.Size
		srcIsSlice = srcElem.IsSlice()
	} else if inputs[1].TypeSignature.Type == ast.TYPE_SLICE_ATOMIC {
		// OBJMODEL: state.CXTypeSignature_ArrayMng.CXTypeSignature_ArrayGet(gid), phase 5 #317.
		// Now an arena getter, site S1340.
		sliceDetails := prgrm.GetCXTypeSignatureArrayFromArray(inputs[1].TypeSignature.Meta)

		srcType = types.Code(sliceDetails.Meta)
		srcSize = types.Code(sliceDetails.Meta).Size()
		srcIsSlice = true
	} else {
		panic("type is not type cx argument deprecate\n\n")
	}

	fp := inputs[0].FramePointer

	dstOffset := ast.GetSliceOffset(prgrm, fp, inputs[0].TypeSignature)
	srcOffset := ast.GetSliceOffset(prgrm, fp, inputs[1].TypeSignature)

	if dstType != srcType || !dstIsSlice || !srcIsSlice || dstSize != srcSize {
		panic(types.CX_RUNTIME_INVALID_ARGUMENT)
	}

	dst := ast.GetSliceData(prgrm, dstOffset, dstSize)
	src := ast.GetSliceData(prgrm, srcOffset, srcSize)

	var count types.Pointer
	if dstType == srcType {
		count = types.Cast_int_to_ptr(copy(dst, src))
		if count%dstSize != 0 {
			panic(types.CX_RUNTIME_ERROR)
		}
	} else {
		panic(types.CX_RUNTIME_INVALID_ARGUMENT)
	}

	outputs[0].Set_i32(prgrm, types.Cast_ptr_to_i32(count/dstSize)) // TODO:PTR use ptr instead of i32
}
