// Package gc is the heap collector of the old loop. It sees a root table and
// the heap, and nothing else of the program. See docs/GarbageCollector.md.
package gc

import (
	"slices"

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

// GCType describes where a value holds pointers, slices and strs. A nil
// *GCType holds none.
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
// offset, flagged with ESCAPE_OFFSET_FLAG when the local escaped, and a
// global root an address. GCRootInner is added after the frame address.
type GCRoot struct {
	GCRootOffset types.Pointer
	GCRootInner  types.Pointer
	GCRootType   *GCType
}

// GCRootFrame is a live frame, its frame pointer and the roots of its
// function.
type GCRootFrame struct {
	GCRootFramePointer types.Pointer
	GCRootFrameRoots   []GCRoot
}

// GCRootTable is everything the collection reads besides the heap: the
// roots of the globals, the live frames and the stack size, below which a
// frame offset is relative to its frame.
type GCRootTable struct {
	GCRootTableGlobals   []GCRoot
	GCRootTableFrames    []GCRootFrame
	GCRootTableStackSize types.Pointer
}

// GCVisit is a pointer or slice the mark followed, and its GCType.
type GCVisit struct {
	GCVisitAddress types.Pointer
	GCVisitType    *GCType
}

// GCState is the scratch memory of the collection, kept between
// collections so a collection allocates only when it outgrows it.
type GCState struct {
	// GCStateObjectStarts lists the address of each object.
	GCStateObjectStarts []types.Pointer
	// GCStateSlots lists the address of each reference the mark read.
	GCStateSlots []types.Pointer
	// GCStateVisited holds the pointers and slices the mark followed.
	GCStateVisited map[GCVisit]bool
	// GCStateWork holds the pointers and slices the mark has yet to follow.
	GCStateWork []GCVisit
}

// gcCollection is one collection of the heap from heapStartsAt to
// heapStartsAt+heapPointer in memory.
type gcCollection struct {
	memory       []byte
	heapStartsAt types.Pointer
	heapPointer  types.Pointer
	state        *GCState
}

// GarbageCollectorCollect collects the heap of memory that starts at
// heapStartsAt and is used up to heapStartsAt+heapPointer. It marks every
// object reachable from roots, walking each root by its GCType and
// recording each reference it reads, then gives each live object its new
// address, moves the recorded references to the new addresses and slides
// the objects down. It returns the new heap pointer.
// VIOLATION VMALLOC-011: the CX collector grows its Go scratch to the largest heap so far, #407.
func GarbageCollectorCollect(memory []byte, heapStartsAt types.Pointer, heapPointer types.Pointer, roots *GCRootTable, state *GCState) types.Pointer {
	collection := gcCollection{memory: memory, heapStartsAt: heapStartsAt, heapPointer: heapPointer, state: state}
	collection.gcObjectStartsBuild()
	if state.GCStateVisited == nil {
		state.GCStateVisited = map[GCVisit]bool{}
	}
	clear(state.GCStateVisited)
	state.GCStateSlots = state.GCStateSlots[:0]
	state.GCStateWork = state.GCStateWork[:0]

	for _, root := range roots.GCRootTableGlobals {
		collection.gcMarkValue(root.GCRootOffset, root.GCRootType)
	}
	for _, frame := range roots.GCRootTableFrames {
		for _, root := range frame.GCRootFrameRoots {
			if address, found := collection.gcFrameAddress(roots, frame.GCRootFramePointer, root.GCRootOffset); found {
				collection.gcMarkValue(address+root.GCRootInner, root.GCRootType)
			}
		}
	}
	collection.gcMarkDrain()

	forward := constants.NULL_HEAP_ADDRESS_OFFSET
	for _, object := range state.GCStateObjectStarts {
		if memory[object] == 1 {
			types.Write_obj_forwarding_address(memory, object, heapStartsAt+forward)
			forward += types.Read_obj_size(memory, object)
		}
	}

	slices.Sort(state.GCStateSlots)
	for i, slot := range state.GCStateSlots {
		if i > 0 && slot == state.GCStateSlots[i-1] {
			continue
		}
		pointer := types.Read_ptr(memory, slot)
		if object, found := collection.gcObjectContaining(pointer); found {
			types.Write_ptr(memory, slot, pointer-object+types.Read_obj_forwarding_address(memory, object))
		}
	}

	for _, object := range state.GCStateObjectStarts {
		if memory[object] != 1 {
			continue
		}
		memory[object] = 0
		size := types.Read_obj_size(memory, object)
		to := types.Read_obj_forwarding_address(memory, object)
		copy(memory[to:to+size], memory[object:object+size])
	}
	return forward
}

// gcMarkValue marks what the value of GCType t at address refers to.
func (collection *gcCollection) gcMarkValue(address types.Pointer, t *GCType) {
	switch t.GCTypeKind {
	case GCKindPointer, GCKindSlice, GCKindStr:
		collection.gcMarkReference(address, t)
	case GCKindArray:
		for i := types.Pointer(0); i < t.GCTypeLength; i++ {
			collection.gcMarkValue(address+i*t.GCTypeElemSize, t.GCTypeElem)
		}
	case GCKindStruct:
		for _, field := range t.GCTypeFields {
			collection.gcMarkValue(address+field.GCTypeFieldOffset, field.GCTypeFieldType)
		}
	}
}

// gcMarkReference marks the object the pointer, slice or str at slot points
// into, records slot for the update, and queues what the object holds.
// VIOLATION VMALLOC-011: the CX collector grows its Go scratch to the largest heap so far, #407.
func (collection *gcCollection) gcMarkReference(slot types.Pointer, t *GCType) {
	if uint64(slot)+uint64(types.POINTER_SIZE) > uint64(len(collection.memory)) {
		return
	}
	pointer := types.Read_ptr(collection.memory, slot)
	if pointer < collection.heapStartsAt+constants.NULL_HEAP_ADDRESS_OFFSET {
		return
	}
	object, found := collection.gcObjectContaining(pointer)
	if !found {
		return
	}
	state := collection.state
	state.GCStateSlots = append(state.GCStateSlots, slot)
	types.Write_obj_mark(collection.memory, object, 1)
	if t.GCTypeElem == nil {
		return
	}
	visit := GCVisit{GCVisitAddress: pointer, GCVisitType: t}
	if state.GCStateVisited[visit] {
		return
	}
	state.GCStateVisited[visit] = true
	state.GCStateWork = append(state.GCStateWork, visit)
}

// gcMarkDrain marks what the queued pointers and slices refer to. A pointer
// to the heap is the address of the value less the object header.
func (collection *gcCollection) gcMarkDrain() {
	state := collection.state
	for len(state.GCStateWork) > 0 {
		visit := state.GCStateWork[len(state.GCStateWork)-1]
		state.GCStateWork = state.GCStateWork[:len(state.GCStateWork)-1]
		pointer, t := visit.GCVisitAddress, visit.GCVisitType
		if t.GCTypeKind == GCKindPointer {
			collection.gcMarkValue(pointer+types.OBJECT_HEADER_SIZE, t.GCTypeElem)
			continue
		}
		object, _ := collection.gcObjectContaining(pointer)
		elements := pointer + types.OBJECT_HEADER_SIZE + constants.SLICE_HEADER_SIZE
		end := object + types.Read_obj_size(collection.memory, object)
		if elements > end || t.GCTypeElemSize == 0 {
			continue
		}
		length := types.Read_ptr(collection.memory, pointer+types.OBJECT_HEADER_SIZE+types.POINTER_SIZE)
		length = min(length, (end-elements)/t.GCTypeElemSize)
		for i := types.Pointer(0); i < length; i++ {
			collection.gcMarkValue(elements+i*t.GCTypeElemSize, t.GCTypeElem)
		}
	}
}

// gcFrameAddress returns the address of the frame variable at offset of the
// frame at framePointer. It returns false for an escaped local whose
// declaration has not run yet. An offset outside the stack is an address.
func (collection *gcCollection) gcFrameAddress(roots *GCRootTable, framePointer types.Pointer, offset types.Pointer) (types.Pointer, bool) {
	if offset&constants.ESCAPE_OFFSET_FLAG == 0 {
		if offset >= roots.GCRootTableStackSize {
			return offset, true
		}
		return offset + framePointer, true
	}
	object := types.Read_ptr(collection.memory, framePointer+offset&^constants.ESCAPE_OFFSET_FLAG)
	if object < collection.heapStartsAt+constants.NULL_HEAP_ADDRESS_OFFSET || object >= collection.heapStartsAt+collection.heapPointer {
		return 0, false
	}
	return object + types.OBJECT_HEADER_SIZE, true
}

// gcObjectStartsBuild lists the address of each heap object in
// GCStateObjectStarts and clears its mark, which a new object may not.
// VIOLATION VMALLOC-011: the CX collector grows its Go scratch to the largest heap so far, #407.
func (collection *gcCollection) gcObjectStartsBuild() {
	starts := collection.state.GCStateObjectStarts[:0]
	end := collection.heapStartsAt + collection.heapPointer
	for c := collection.heapStartsAt + constants.NULL_HEAP_ADDRESS_OFFSET; c < end; {
		starts = append(starts, c)
		collection.memory[c] = 0
		objSize := types.Read_obj_size(collection.memory, c)
		if objSize == 0 {
			break
		}
		c += objSize
	}
	collection.state.GCStateObjectStarts = starts
}

// gcObjectContaining returns the address of the heap object that pointer
// points into. A pointer inside an object, as &s[1] is, keeps it alive.
func (collection *gcCollection) gcObjectContaining(pointer types.Pointer) (types.Pointer, bool) {
	starts := collection.state.GCStateObjectStarts
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
	if pointer >= start+types.Read_obj_size(collection.memory, start) {
		return 0, false
	}
	return start, true
}
