package ast

import (
	"slices"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// MarkAndCompact collects the heap. It marks every object reachable from the
// globals and the frames, walking each root by its GCType and recording each
// reference it reads, then gives each live object its new address, moves the
// recorded references to the new addresses and slides the objects down.
func MarkAndCompact(prgrm *CXProgram) {
	gcObjectStartsBuild(prgrm)
	heap := &prgrm.Heap
	if heap.Visited == nil {
		heap.Visited = map[HeapVisit]bool{}
	}
	clear(heap.Visited)
	heap.Slots = heap.Slots[:0]
	heap.Work = heap.Work[:0]

	for _, root := range gcGlobalRoots(prgrm) {
		gcMarkValue(prgrm, root.GCRootOffset, root.GCRootType)
	}
	var fp types.Pointer
	for c := types.Pointer(0); c <= prgrm.CallCounter; c++ {
		op := prgrm.CallStack[c].Operator
		// A callback a native pushes onto CallStack has no operator.
		if op == nil {
			continue
		}
		for _, root := range gcFrameRoots(prgrm, op) {
			if address, found := gcFrameAddress(prgrm, fp, root.GCRootOffset); found {
				gcMarkValue(prgrm, address+root.GCRootInner, root.GCRootType)
			}
		}
		fp += op.Size
	}
	if object, found := gcObjectContaining(prgrm, heap.Pinned); found {
		types.Write_obj_mark(prgrm.Memory, object, 1)
	}
	gcMarkDrain(prgrm)

	faddr := constants.NULL_HEAP_ADDRESS_OFFSET
	for _, object := range heap.ObjectStarts {
		if prgrm.Memory[object] == 1 {
			types.Write_obj_forwarding_address(prgrm.Memory, object, heap.StartsAt+faddr)
			faddr += types.Read_obj_size(prgrm.Memory, object)
		}
	}

	if object, found := gcObjectContaining(prgrm, heap.Pinned); found {
		heap.Pinned = heap.Pinned - object + types.Read_obj_forwarding_address(prgrm.Memory, object)
	}
	slices.Sort(heap.Slots)
	for i, slot := range heap.Slots {
		if i > 0 && slot == heap.Slots[i-1] {
			continue
		}
		pointer := types.Read_ptr(prgrm.Memory, slot)
		if object, found := gcObjectContaining(prgrm, pointer); found {
			types.Write_ptr(prgrm.Memory, slot, pointer-object+types.Read_obj_forwarding_address(prgrm.Memory, object))
		}
	}

	for _, object := range heap.ObjectStarts {
		if prgrm.Memory[object] != 1 {
			continue
		}
		prgrm.Memory[object] = 0
		size := types.Read_obj_size(prgrm.Memory, object)
		to := types.Read_obj_forwarding_address(prgrm.Memory, object)
		copy(prgrm.Memory[to:to+size], prgrm.Memory[object:object+size])
	}
	heap.Pointer = faddr
}

// gcMarkValue marks what the value of GCType t at address refers to.
func gcMarkValue(prgrm *CXProgram, address types.Pointer, t *GCType) {
	switch t.GCTypeKind {
	case GCKindPointer, GCKindSlice, GCKindStr:
		gcMarkReference(prgrm, address, t)
	case GCKindArray:
		for i := types.Pointer(0); i < t.GCTypeLength; i++ {
			gcMarkValue(prgrm, address+i*t.GCTypeElemSize, t.GCTypeElem)
		}
	case GCKindStruct:
		for _, field := range t.GCTypeFields {
			gcMarkValue(prgrm, address+field.GCTypeFieldOffset, field.GCTypeFieldType)
		}
	}
}

// gcMarkReference marks the object the pointer, slice or str at slot points
// into, records slot for the update, and queues what the object holds.
func gcMarkReference(prgrm *CXProgram, slot types.Pointer, t *GCType) {
	if uint64(slot)+uint64(types.POINTER_SIZE) > uint64(len(prgrm.Memory)) {
		return
	}
	pointer := types.Read_ptr(prgrm.Memory, slot)
	if pointer < prgrm.Heap.StartsAt+constants.NULL_HEAP_ADDRESS_OFFSET {
		return
	}
	object, found := gcObjectContaining(prgrm, pointer)
	if !found {
		return
	}
	prgrm.Heap.Slots = append(prgrm.Heap.Slots, slot)
	types.Write_obj_mark(prgrm.Memory, object, 1)
	if t.GCTypeElem == nil {
		return
	}
	visit := HeapVisit{HeapVisitAddress: pointer, HeapVisitType: t}
	if prgrm.Heap.Visited[visit] {
		return
	}
	prgrm.Heap.Visited[visit] = true
	prgrm.Heap.Work = append(prgrm.Heap.Work, visit)
}

// gcMarkDrain marks what the queued pointers and slices refer to. A pointer
// to the heap is the address of the value less the object header.
func gcMarkDrain(prgrm *CXProgram) {
	for len(prgrm.Heap.Work) > 0 {
		visit := prgrm.Heap.Work[len(prgrm.Heap.Work)-1]
		prgrm.Heap.Work = prgrm.Heap.Work[:len(prgrm.Heap.Work)-1]
		pointer, t := visit.HeapVisitAddress, visit.HeapVisitType
		if t.GCTypeKind == GCKindPointer {
			gcMarkValue(prgrm, pointer+types.OBJECT_HEADER_SIZE, t.GCTypeElem)
			continue
		}
		object, _ := gcObjectContaining(prgrm, pointer)
		elements := pointer + types.OBJECT_HEADER_SIZE + constants.SLICE_HEADER_SIZE
		end := object + types.Read_obj_size(prgrm.Memory, object)
		length := GetSliceLen(prgrm, pointer)
		if elements > end || t.GCTypeElemSize == 0 {
			continue
		}
		length = min(length, (end-elements)/t.GCTypeElemSize)
		for i := types.Pointer(0); i < length; i++ {
			gcMarkValue(prgrm, elements+i*t.GCTypeElemSize, t.GCTypeElem)
		}
	}
}

// updateDisplaceReference performs the actual addition or subtraction of `plusOff` to the address being pointed by the element at `atOffset`.
func updateDisplaceReference(prgrm *CXProgram, updated *map[types.Pointer]types.Pointer, atOffset types.Pointer, plusOff types.Pointer) {
	// Checking if it was already updated.
	if _, found := (*updated)[atOffset]; found {
		return
	}

	// Extracting the address being pointed by element at `atOffset`
	dsCurrAddr := types.Read_ptr(prgrm.Memory, atOffset)

	// Adding `plusOff` to the address and updating the address pointed by
	// element at `atOffset`.
	types.Write_ptr(prgrm.Memory, atOffset, dsCurrAddr+plusOff)

	// Keeping a record of this address. We don't want to displace the object twice.
	// We're using a map to speed things up a tiny bit.
	(*updated)[atOffset] = atOffset
}

// doDisplaceReferences checks if the element at `atOffset` is pointing to an object on the heap and, if this is the case, it displaces it by `plusOff`. `updated` keeps a record of all the offsets that have already been updated.
func doDisplaceReferences(prgrm *CXProgram, updated *map[types.Pointer]types.Pointer, atOffset types.Pointer, plusOff types.Pointer, baseType types.Code, declSpecs []int) {
	var numDeclSpecs = len(declSpecs)

	// Getting the offset to the object in the heap.
	heapOffset := types.Read_ptr(prgrm.Memory, atOffset)

	// The whole displacement process is needed because the objects on the heap were
	// displaced by additional data segment bytes. These additional bytes need to be
	// considered if we want to read the objects on the heap. We need to check if the
	// displacement is positive or negative; if it is positive then this means a data
	// segment was added; if it is negative it means that we're done with any CX chains
	// process involving the addition of a data segment, and in some code snippets we can ignore the displacement.
	// TODO: Maybe this condition can be avoided by refactoring the code?
	var condPlusOff types.Pointer
	if plusOff > 0 {
		condPlusOff = plusOff
	}

	// Displace the address pointed by element at `atOffset`.
	updateDisplaceReference(prgrm, updated, atOffset, plusOff)

	// It can't be a tree of objects.
	if numDeclSpecs == 0 || heapOffset <= prgrm.Heap.StartsAt+condPlusOff {
		return
	}

	// Checking if it's a tree of objects.
	// TODO: We're not considering struct instances with pointer fields.
	if declSpecs[0] == constants.DECL_SLICE {
		if (numDeclSpecs > 1 &&
			(declSpecs[1] == constants.DECL_SLICE ||
				declSpecs[1] == constants.DECL_POINTER)) ||
			(numDeclSpecs == 1 && baseType == types.STR) {
			// Then we need to iterate each of the slice objects
			// and check if we need to update their address.
			sliceLen := GetSliceLen(prgrm, heapOffset+condPlusOff)

			offsetToElements := types.OBJECT_HEADER_SIZE + constants.SLICE_HEADER_SIZE

			for c := types.Pointer(0); c < sliceLen; c++ {
				cHeapOffset := types.Read_ptr(prgrm.Memory, heapOffset+condPlusOff+offsetToElements+c*types.POINTER_SIZE)
				if cHeapOffset <= prgrm.Heap.StartsAt+condPlusOff {
					// Then it's pointing to null or data segment
					continue
				}

				// Displacing this child element.
				updateDisplaceReference(prgrm, updated, heapOffset+offsetToElements+c*types.POINTER_SIZE, plusOff)
			}
		}
	}
}

// DisplaceReferences displaces all the pointer-like variables, slice elements or field structures by `off`. `numPkgs` tells us the number of packages to consider for the reference desplacement (this number should equal to the number of packages that represent the blockchain code in a CX chain).
func DisplaceReferences(prgrm *CXProgram, off types.Pointer, numPkgs int) {
	// We're going to keep a record of all the references that were already updated.
	updated := make(map[types.Pointer]types.Pointer)

	count := 0
	for _, pkgIdx := range prgrm.Packages {
		// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
		// Now an arena getter, site S0347.
		pkg, err := prgrm.GetPackageFromArray(pkgIdx)
		if err != nil {
			panic(err)
		}

		if count > numPkgs {
			break
		}

		// In a CX chain we're only interested on considering global variables,
		// as any other object should be destroyed, as the program finished its
		// execution.
		for _, glblFldIdx := range pkg.Globals.Fields {
			// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
			// Now an arena getter, site S0348.
			glblFld := prgrm.GetCXTypeSignatureFromArray(glblFldIdx)

			var glbl *CXArgument
			if glblFld.Type == TYPE_CXARGUMENT_DEPRECATE {
				// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
				// Now an arena getter, site S0350.
				glbl = prgrm.GetCXArg(CXArgumentIndex(glblFld.Meta))
			} else {
				panic("type is not cxargument deprecate.")
			}

			if glbl.IsString() || glbl.IsStruct() || glbl.IsSlice() {
				doDisplaceReferences(prgrm, &updated, glbl.Offset, off, glbl.Type, glbl.DeclarationSpecifiers[1:]) // TODO:PTR remove hardcoded offsets
			}

			// If it's a struct instance we need to displace each of its fields.
			if glbl.StructType != nil {
				for fieldIndex, typeSignatureIdx := range glbl.StructType.Fields {
					// OBJMODEL: state.CXTypeSignatureMng.CXTypeSignatureGet(gid), phase 5 #332.
					// Now an arena getter, site S0351.
					typeSignature := prgrm.GetCXTypeSignatureFromArray(typeSignatureIdx)

					var fld *CXArgument
					if typeSignature.Type == TYPE_CXARGUMENT_DEPRECATE {
						fldIdx := typeSignature.Meta
						// OBJMODEL: state.CXArgumentMng.CXArgumentGet(gid), phase 5 #327.
						// Now an arena getter, site S0353.
						fld = prgrm.GetCXArg(CXArgumentIndex(fldIdx))
					} else {
						fld = typeSignature.GetCXArgFormat(prgrm)
					}

					if fld.IsString() || fld.IsStruct() || fld.IsSlice() {
						doDisplaceReferences(prgrm, &updated, glbl.Offset+StructFieldOffset(prgrm, glbl.StructType, fieldIndex), off, fld.Type, fld.DeclarationSpecifiers[1:])
					}
				}
			}
		}

		count++
	}
}

// gcFrameAddress returns the address of the frame variable at offset. It
// returns false for an escaped local whose declaration has not run yet. An
// offset outside the stack is an address already, as frameAddress has it.
func gcFrameAddress(prgrm *CXProgram, fp types.Pointer, offset types.Pointer) (types.Pointer, bool) {
	if !OffsetEscaped(offset) {
		if offset >= prgrm.Stack.Size {
			return offset, true
		}
		return offset + fp, true
	}
	object := types.Read_ptr(prgrm.Memory, fp+OffsetEscapedSlot(offset))
	if object < prgrm.Heap.StartsAt+constants.NULL_HEAP_ADDRESS_OFFSET || object >= prgrm.Heap.StartsAt+prgrm.Heap.Pointer {
		return 0, false
	}
	return object + types.OBJECT_HEADER_SIZE, true
}

// gcObjectStartsBuild lists the address of each heap object in
// prgrm.Heap.ObjectStarts and clears its mark, which a new object may not.
func gcObjectStartsBuild(prgrm *CXProgram) {
	starts := prgrm.Heap.ObjectStarts[:0]
	for c := prgrm.Heap.StartsAt + constants.NULL_HEAP_ADDRESS_OFFSET; c < prgrm.Heap.StartsAt+prgrm.Heap.Pointer; {
		starts = append(starts, c)
		prgrm.Memory[c] = 0
		objSize := types.Read_obj_size(prgrm.Memory, c)
		if objSize == 0 {
			break
		}
		c += objSize
	}
	prgrm.Heap.ObjectStarts = starts
}

// gcObjectContaining returns the address of the heap object that pointer
// points into. A pointer inside an object, as &s[1] is, keeps it alive.
func gcObjectContaining(prgrm *CXProgram, pointer types.Pointer) (types.Pointer, bool) {
	starts := prgrm.Heap.ObjectStarts
	low, high := 0, len(starts)
	for low < high {
		middle := int(uint(low+high) >> 1)
		if starts[middle] <= pointer {
			low = middle + 1
		} else {
			high = middle
		}
	}
	if low == 0 {
		return 0, false
	}
	start := starts[low-1]
	if pointer >= start+types.Read_obj_size(prgrm.Memory, start) {
		return 0, false
	}
	return start, true
}
