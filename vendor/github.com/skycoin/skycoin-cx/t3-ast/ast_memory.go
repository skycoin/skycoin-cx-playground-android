package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// MinHeapSize is the initial heap of a program with the heap cap
// MAX_HEAP_SIZE, see MinHeapSizeOf.
func MinHeapSize() types.Pointer {
	return MinHeapSizeOf(constants.MAX_HEAP_SIZE)
}

// MinHeapSizeOf is the initial heap of a program with the heap cap heapCap:
// INIT_HEAP_SIZE, at most heapCap and at least NULL_HEAP_ADDRESS_OFFSET,
// the bytes of nil.
func MinHeapSizeOf(heapCap types.Pointer) types.Pointer {
	return max(min(constants.INIT_HEAP_SIZE, heapCap), constants.NULL_HEAP_ADDRESS_OFFSET)
}

// HeapCap returns the heap cap of prgrm, Heap.Max, or MAX_HEAP_SIZE for a
// program that has none.
func HeapCap(prgrm *CXProgram) types.Pointer {
	if prgrm.Heap.Max == 0 {
		return constants.MAX_HEAP_SIZE
	}
	return prgrm.Heap.Max
}

// EnsureMinimumHeapSize sizes the memory of `prgrm` before a run. It
// allocates the memory for the heap cap once, with the scratch and root
// table of the collector (D42), gives the heap `MinHeapSizeOf` bytes after
// the data segment, and starts the GC pressure from 0. It allocates nothing
// for a program it has sized.
// GCREACH: sets the GC pressure and its limit and prepares the collector,
// see docs/GarbageCollector.md.
func (cxprogram *CXProgram) EnsureMinimumHeapSize() {
	heap := &cxprogram.Heap
	heap.Max = min(HeapCap(cxprogram), constants.ESCAPE_OFFSET_FLAG/2)
	heapMemoryReserve(cxprogram)
	currHeapSize := types.Cast_int_to_ptr(len(cxprogram.Memory)) - heap.StartsAt
	if minHeapSize := MinHeapSizeOf(heap.Max); currHeapSize < minHeapSize {
		length := len(cxprogram.Memory)
		cxprogram.Memory = cxprogram.Memory[:heap.StartsAt+minHeapSize]
		clear(cxprogram.Memory[length:])
		heap.Size = minHeapSize
	}
	GarbageCollectorPrepare(cxprogram)
	heap.Pressure = 0
	heap.PressureLimit = heapPressureLimit(cxprogram)
}

// heapMemoryReserve gives prgrm.Memory the capacity of the stack, the data
// and the heap cap, which the heap grows into without allocating.
func heapMemoryReserve(prgrm *CXProgram) {
	size := max(prgrm.Heap.StartsAt+HeapCap(prgrm), types.Cast_int_to_ptr(len(prgrm.Memory)))
	if types.Cast_int_to_ptr(cap(prgrm.Memory)) < size {
		memory := make([]byte, len(prgrm.Memory), size)
		copy(memory, prgrm.Memory)
		prgrm.Memory = memory
	}
}

// ResizeMemory sets the heap of prgrm to newMemSize bytes, at most its cap.
// The heap grows into the capacity of prgrm.Memory, which zeroes the new
// bytes, and shrinks by reslicing.
func ResizeMemory(prgrm *CXProgram, newMemSize types.Pointer, isExpand bool) {
	heapCap := HeapCap(prgrm)
	if newMemSize > heapCap {
		newMemSize = heapCap
	}

	if newMemSize == prgrm.Heap.Size {
		// Then we're at the limit; we can't expand anymore.
		// We can only hope that the free memory is enough for the CX program to continue running.
		return
	}

	end := prgrm.Heap.StartsAt + newMemSize
	if isExpand {
		if types.Cast_int_to_ptr(cap(prgrm.Memory)) < end {
			// A program that EnsureMinimumHeapSize did not size.
			heapMemoryReserve(prgrm)
		}
		length := len(prgrm.Memory)
		prgrm.Memory = prgrm.Memory[:end]
		clear(prgrm.Memory[length:])
	} else {
		prgrm.Memory = prgrm.Memory[:end]
	}
	prgrm.Heap.Size = newMemSize
}

// AllocateSeq allocates size bytes on the heap and returns their address.
// It never collects. When the heap is full it grows it, up to the heap
// cap, and past that panics with RuntimeOutOfMemory. The bytes count toward
// Heap.Pressure, which the next GarbageCollectorCheck reads.
// GCREACH: grows the heap and adds to the GC pressure,
// see docs/GarbageCollector.md.
func AllocateSeq(prgrm *CXProgram, size types.Pointer) types.Pointer {
	address := prgrm.Heap.Pointer
	free := address + size
	if free > prgrm.Heap.Size {
		if free > HeapCap(prgrm) || free < address {
			HeapOutOfMemory(prgrm, uint64(size))
		}
		// Growing to leave MIN_HEAP_FREE_RATIO free keeps the next
		// allocations from growing it again before the next collection.
		grown := max(free, types.Cast_f32_to_ptr(float32(free)/(1.0-constants.MIN_HEAP_FREE_RATIO)))
		ResizeMemory(prgrm, grown, true)
	}
	prgrm.Heap.Pointer = free
	prgrm.Heap.Pressure += size
	return address + prgrm.Heap.StartsAt
}

// HeapOutOfMemory stops the run with an out of memory error for an
// allocation of request bytes that does not fit the heap cap.
func HeapOutOfMemory(prgrm *CXProgram, request uint64) {
	panic(RuntimeOutOfMemory{RuntimeOutOfMemoryRequest: request, RuntimeOutOfMemoryTaken: prgrm.Heap.Pointer, RuntimeOutOfMemoryCap: HeapCap(prgrm)})
}

// AllocateStr returns a new str object holding data. It writes the object
// after the allocation, which may give prgrm a new Memory.
// GCREACH: allocates through AllocateSeq, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-281: B, only the boundary opcodes use Go strings, #412.
func AllocateStr(prgrm *CXProgram, data string) types.Pointer {
	size := types.OBJECT_HEADER_SIZE + types.Cast_int_to_ptr(len(data))
	object := AllocateSeq(prgrm, size)
	types.Write_obj_size(prgrm.Memory, object, size)
	copy(prgrm.Memory[object+types.OBJECT_HEADER_SIZE:object+size], data)
	return object
}

// AllocateStrAt writes a new str object holding data and stores its address
// at address. Nothing collects inside it, so address stays valid.
// GCREACH: allocates through AllocateSeq, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-282: B, only the boundary opcodes use Go strings, #412.
func AllocateStrAt(prgrm *CXProgram, address types.Pointer, data string) {
	types.Write_ptr(prgrm.Memory, address, AllocateStr(prgrm, data))
}

// AllocateStrBytesAt writes a new str object holding first followed by second
// and stores its address at address. Both may be views into prgrm.Memory,
// which keeps its array while the heap grows.
// GCREACH: allocates through AllocateSeq, which adds to the GC pressure,
// see docs/GarbageCollector.md.
func AllocateStrBytesAt(prgrm *CXProgram, address types.Pointer, first []byte, second []byte) {
	size := types.OBJECT_HEADER_SIZE + types.Cast_int_to_ptr(len(first)+len(second))
	object := AllocateSeq(prgrm, size)
	types.Write_obj_size(prgrm.Memory, object, size)
	data := prgrm.Memory[object+types.OBJECT_HEADER_SIZE : object+size]
	copy(data[copy(data, first):], second)
	types.Write_ptr(prgrm.Memory, address, object)
}

// heapPressureLimit is the bytes the program may allocate before the next
// collection: the free heap, but no more than half of what is left below the
// heap cap, so that a heap near its cap collects before the heap is full
// rather than stopping with an out of memory error.
// GCREACH: sets the GC pressure limit, see docs/GarbageCollector.md.
func heapPressureLimit(prgrm *CXProgram) types.Pointer {
	heap := &prgrm.Heap
	room := HeapCap(prgrm) - min(heap.Pointer, HeapCap(prgrm))
	return min(heap.Size-heap.Pointer, room/2)
}
