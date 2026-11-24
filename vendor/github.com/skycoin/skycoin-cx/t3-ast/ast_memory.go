package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// MinHeapSize determines what's the minimum heap size that a CX program
// needs to have based on INIT_HEAP_SIZE, MAX_HEAP_SIZE and NULL_HEAP_ADDRESS_OFFSET.
func MinHeapSize() types.Pointer {
	minHeapSize := constants.INIT_HEAP_SIZE
	if constants.MAX_HEAP_SIZE < constants.INIT_HEAP_SIZE {
		// Then MAX_HEAP_SIZE overrides INIT_HEAP_SIZE's value.
		minHeapSize = constants.MAX_HEAP_SIZE
	}
	if minHeapSize < constants.NULL_HEAP_ADDRESS_OFFSET {
		// Then the user is trying to allocate too little heap memory.
		// We need at least NULL_HEAP_ADDRESS_OFFSET bytes for `nil`.
		minHeapSize = constants.NULL_HEAP_ADDRESS_OFFSET
	}

	return minHeapSize
}

// EnsureMinimumHeapSize ensures that `prgrm` has `minHeapSize()` bytes
// allocated after the data segment, and starts the GC pressure from 0.
// GCREACH: sets the GC pressure and its limit,
// see docs/GarbageCollector.md.
func (cxprogram *CXProgram) EnsureMinimumHeapSize() {
	currHeapSize := types.Cast_int_to_ptr(len(cxprogram.Memory)) - cxprogram.Heap.StartsAt
	minHeapSize := MinHeapSize()
	if currHeapSize < minHeapSize {
		cxprogram.Memory = append(cxprogram.Memory, make([]byte, minHeapSize-currHeapSize)...)
		cxprogram.Heap.Size = minHeapSize
	}
	cxprogram.Heap.Pressure = 0
	cxprogram.Heap.PressureLimit = cxprogram.Heap.Size - cxprogram.Heap.Pointer
}

// ResizeMemory ...
func ResizeMemory(prgrm *CXProgram, newMemSize types.Pointer, isExpand bool) {
	// We can't expand memory to a value greater than `memLimit`.
	if newMemSize > constants.MAX_HEAP_SIZE {
		newMemSize = constants.MAX_HEAP_SIZE
	}

	if newMemSize == prgrm.Heap.Size {
		// Then we're at the limit; we can't expand anymore.
		// We can only hope that the free memory is enough for the CX program to continue running.
		return
	}

	if isExpand {
		// Adding bytes to reach a heap equal to `newMemSize`.
		prgrm.Memory = append(prgrm.Memory, make([]byte, newMemSize-prgrm.Heap.Size)...)
		prgrm.Heap.Size = newMemSize
	} else {
		// Reslicing keeps the array for the next growth. The copy that
		// releases it runs only when it would free more than half.
		prgrm.Memory = prgrm.Memory[:prgrm.Heap.StartsAt+newMemSize]
		if cap(prgrm.Memory) > 2*len(prgrm.Memory) {
			prgrm.Memory = append([]byte(nil), prgrm.Memory...)
		}
		prgrm.Heap.Size = newMemSize
	}
}

// AllocateSeq allocates size bytes on the heap and returns their address.
// It never collects. When the heap is full it grows it, up to
// MAX_HEAP_SIZE, and past that panics with HEAP_EXHAUSTED_ERROR. The bytes
// count toward Heap.Pressure, which the next GarbageCollectorCheck reads.
// GCREACH: grows the heap and adds to the GC pressure,
// see docs/GarbageCollector.md.
func AllocateSeq(prgrm *CXProgram, size types.Pointer) types.Pointer {
	address := prgrm.Heap.Pointer
	free := address + size
	if free > prgrm.Heap.Size {
		if free > constants.MAX_HEAP_SIZE || free < address {
			panic(constants.HEAP_EXHAUSTED_ERROR)
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

// AllocateStr returns a new str object holding data. It writes the object
// after the allocation, which may give prgrm a new Memory.
// GCREACH: allocates through AllocateSeq, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-281: O, the opcode works on Go string copies of CX strs, #412.
func AllocateStr(prgrm *CXProgram, data string) types.Pointer {
	size := types.OBJECT_HEADER_SIZE + types.Cast_int_to_ptr(len(data))
	object := AllocateSeq(prgrm, size)
	types.Write_obj_data(prgrm.Memory, object, size, []byte(data))
	return object
}

// AllocateStrAt writes a new str object holding data and stores its address
// at address. Nothing collects inside it, so address stays valid.
// GCREACH: allocates through AllocateSeq, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-282: O, the opcode works on Go string copies of CX strs, #412.
func AllocateStrAt(prgrm *CXProgram, address types.Pointer, data string) {
	types.Write_ptr(prgrm.Memory, address, AllocateStr(prgrm, data))
}
