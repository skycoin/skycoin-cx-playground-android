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

// GarbageCollectorTrigger collects the heap at a safe point of the old loop,
// between two steps, where every root is in a global or a frame slot. It
// hands the collector a root table and the heap, then resizes the heap to
// keep MIN_HEAP_FREE_RATIO to MAX_HEAP_FREE_RATIO of it free.
// GCREACH: builds the root table and runs the collector,
// see docs/GarbageCollector.md.
// VIOLATION VMALLOC-011: the CX collector builds its root table and type cache in Go memory, #407.
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
	heap.PressureLimit = heap.Size - heap.Pointer
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
