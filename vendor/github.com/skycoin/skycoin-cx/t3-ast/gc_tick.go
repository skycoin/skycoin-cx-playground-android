package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	gc "github.com/skycoin/skycoin-cx/t2-cx-runtime-gc"
)

// GarbageCollectorCheck reports whether the heap has taken more bytes since
// the last collection than it had free then. The old loop calls it once per
// step, so it reads two fields and allocates nothing.
// GCREACH: reads the GC pressure, see docs/GarbageCollector.md.
func GarbageCollectorCheck(prgrm *CXProgram) bool {
	return prgrm.Heap.Pressure > prgrm.Heap.PressureLimit
}

// GarbageCollectorPrepare builds the roots of the globals and of every
// function and allocates the scratch of the collector and the frames of the
// root table for the heap cap, once before the run (D42). It allocates
// nothing for a program it prepared.
// GCREACH: builds the root table and the collector scratch,
// see docs/GarbageCollector.md.
func GarbageCollectorPrepare(prgrm *CXProgram) {
	heap := &prgrm.Heap
	cache := gcTypeCache(prgrm)
	gcGlobalRoots(prgrm)
	if cache.GCTypeCacheFunctions != len(prgrm.CXFunctions) {
		for i := range prgrm.CXFunctions {
			gcFrameRoots(prgrm, &prgrm.CXFunctions[i])
		}
		cache.GCTypeCacheFunctions = len(prgrm.CXFunctions)
	}
	if cap(heap.Roots.GCRootTableFrames) < len(prgrm.CallStack) {
		heap.Roots.GCRootTableFrames = make([]gc.GCRootFrame, 0, len(prgrm.CallStack))
	}
	memorySize := types.Cast_int_to_ptr(cap(prgrm.Memory))
	gc.GCStatePrepare(&heap.Collector, memorySize-heap.StartsAt, memorySize)
}

// GarbageCollectorTrigger collects the heap at a safe point of the old loop,
// between two steps, where every root is in a global or a frame slot. It
// hands the collector a root table and the heap, then resizes the heap to
// keep MIN_HEAP_FREE_RATIO to MAX_HEAP_FREE_RATIO of it free. It allocates
// nothing, as GarbageCollectorPrepare built the roots and the scratch.
// GCREACH: builds the root table and runs the collector,
// see docs/GarbageCollector.md.
func GarbageCollectorTrigger(prgrm *CXProgram) {
	heap := &prgrm.Heap
	roots := &heap.Roots
	roots.GCRootTableGlobals = gcGlobalRoots(prgrm)
	roots.GCRootTableStackSize = prgrm.Stack.Size
	frames := roots.GCRootTableFrames[:0]
	// After the last return CallCounter is invalid and no frame is live.
	for c := types.Pointer(0); prgrm.CallCounter.IsValid() && c <= prgrm.CallCounter; c++ {
		call := &prgrm.CallStack[c]
		// A callback a native pushes onto CallStack has no operator.
		if call.Operator == nil {
			continue
		}
		frames = append(frames, gc.GCRootFrame{GCRootFramePointer: call.FramePointer, GCRootFrameRoots: gcFrameRoots(prgrm, call.Operator)})
	}
	roots.GCRootTableFrames = frames
	heap.Pointer = gc.GarbageCollectorCollect(prgrm.Memory, heap.StartsAt, heap.Pointer, roots, &heap.Collector)
	heapFreeRatioKeep(prgrm)
	heap.Pressure = 0
	heap.PressureLimit = heapPressureLimit(prgrm)
}

// heapFreeRatioKeep grows the heap when less than MIN_HEAP_FREE_RATIO of it
// is free and shrinks it, never below INIT_HEAP_SIZE, when more than
// MAX_HEAP_FREE_RATIO is.
func heapFreeRatioKeep(prgrm *CXProgram) {
	used := prgrm.Heap.Pointer
	free := 1.0 - float32(used)/float32(prgrm.Heap.Size)
	if free < constants.MIN_HEAP_FREE_RATIO {
		// GCREACH: grows the heap after a collection,
		// see docs/GarbageCollector.md.
		ResizeMemory(prgrm, types.Cast_f32_to_ptr(float32(used)/(1.0-constants.MIN_HEAP_FREE_RATIO)), true)
	}
	if free > constants.MAX_HEAP_FREE_RATIO {
		size := types.Cast_f32_to_ptr(float32(used) / (1.0 - constants.MAX_HEAP_FREE_RATIO))
		if size > constants.INIT_HEAP_SIZE {
			// GCREACH: shrinks the heap after a collection,
			// see docs/GarbageCollector.md.
			ResizeMemory(prgrm, size, false)
		}
	}
}
