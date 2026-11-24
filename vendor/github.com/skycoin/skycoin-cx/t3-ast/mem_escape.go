package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// An escaped local has its frame slot offset with ESCAPE_OFFSET_FLAG set, and
// the slot holds its heap object. Every use copies the offset and reads through it.

// OffsetEscaped reports whether offset is the offset of an escaped local.
func OffsetEscaped(offset types.Pointer) bool {
	return offset&constants.ESCAPE_OFFSET_FLAG != 0
}

// OffsetEscapedSlot returns the frame offset of the slot of an escaped local.
func OffsetEscapedSlot(offset types.Pointer) types.Pointer {
	return offset &^ constants.ESCAPE_OFFSET_FLAG
}

// frameAddress returns the address of the variable at offset in the frame at
// fp. An offset outside the stack is an address already.
func frameAddress(prgrm *CXProgram, fp types.Pointer, offset types.Pointer) types.Pointer {
	if OffsetEscaped(offset) {
		return types.Read_ptr(prgrm.Memory, fp+OffsetEscapedSlot(offset)) + types.OBJECT_HEADER_SIZE
	}
	if offset < prgrm.Stack.Size {
		return offset + fp
	}
	return offset
}

// FrameAddress is frameAddress for the GC and the error printer.
func FrameAddress(prgrm *CXProgram, fp types.Pointer, offset types.Pointer) types.Pointer {
	return frameAddress(prgrm, fp, offset)
}

// finalOffsetEscapedPointer is GetFinalOffset of an escaped *T variable.
func finalOffsetEscapedPointer(prgrm *CXProgram, fp types.Pointer, argTypeSig *CXTypeSignature) types.Pointer {
	finalOffset := frameAddress(prgrm, fp, argTypeSig.Offset)
	if !argTypeSig.IsDeref {
		return finalOffset
	}
	finalOffset = types.Read_ptr(prgrm.Memory, finalOffset)
	if finalOffset.IsValid() && finalOffset >= prgrm.Heap.StartsAt {
		finalOffset += types.OBJECT_HEADER_SIZE
	}
	return finalOffset
}

// EscapedAllocate gives the escaped local at offset in the frame at fp a new
// zeroed heap object of size bytes, as its declaration runs.
// GCREACH: allocates the object of an escaped local, which adds to
// the GC pressure, see docs/GarbageCollector.md.
func EscapedAllocate(prgrm *CXProgram, fp types.Pointer, offset types.Pointer, size types.Pointer) {
	objectSize := types.OBJECT_HEADER_SIZE + size
	object := AllocateSeq(prgrm, objectSize)
	header := types.Get_obj_header(prgrm.Memory, object)
	for c := range header {
		header[c] = 0
	}
	types.Write_obj_size(prgrm.Memory, object, objectSize)
	data := types.Get_obj_data(prgrm.Memory, object, size)
	for c := range data {
		data[c] = 0
	}
	types.Write_ptr(prgrm.Memory, fp+OffsetEscapedSlot(offset), object)
}

// PointerOfAddress returns the pointer to the value at address. A pointer to
// the heap is the address less the object header, which a dereference adds.
func PointerOfAddress(prgrm *CXProgram, address types.Pointer) types.Pointer {
	if address.IsValid() && address >= prgrm.Heap.StartsAt+types.OBJECT_HEADER_SIZE {
		return address - types.OBJECT_HEADER_SIZE
	}
	return address
}

// escapedSize is the size of the heap object of an escaped local, the size
// its frame slot would have had.
func escapedSize(prgrm *CXProgram, sig *CXTypeSignature) types.Pointer {
	if sig.Type == TYPE_CXARGUMENT_DEPRECATE {
		return GetArgSize(prgrm, prgrm.GetCXArg(CXArgumentIndex(sig.Meta)))
	}
	return sig.GetSize(prgrm, true)
}
