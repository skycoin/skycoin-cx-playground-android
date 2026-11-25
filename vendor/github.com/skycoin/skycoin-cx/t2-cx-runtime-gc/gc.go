// Package gc is the heap collector of the old loop. It sees a root table and
// the heap, and nothing else of the program. See docs/GarbageCollector.md.
package gc

import (
	"math/bits"

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
	// GCTypeIndex is the position of a pointer or slice type in
	// GCStateTypes plus one, set by GCStateTypeAdd. 0 is not added.
	GCTypeIndex types.Pointer
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

// GCVisit is a pointer the mark has yet to follow, and its GCType.
type GCVisit struct {
	GCVisitAddress types.Pointer
	GCVisitType    *GCType
}

// The mark byte of an object header. gcMarkPending is an object the mark
// queued when GCStateWork was full, found again by a scan of the heap.
const (
	gcMarkLive    = 1
	gcMarkPending = 2
)

// GCState is the scratch memory of the collection. GCStatePrepare
// allocates it once for the heap cap, so a collection allocates nothing.
type GCState struct {
	// GCStateStarts has a bit for each byte of the heap, set where an
	// object starts.
	GCStateStarts []uint64
	// GCStateSlots has a bit for each byte of memory, set where the mark
	// read a reference. Its set bits are the references to update, in
	// address order.
	GCStateSlots []uint64
	// GCStateSlotWords has a bit for each word of GCStateSlots that holds a
	// set bit, and GCStateSlotHigh is the highest such word.
	GCStateSlotWords []uint64
	GCStateSlotHigh  int
	// GCStateWork holds heap objects to walk with the type that the
	// forwarding address field of their header names while the mark runs.
	GCStateWork []types.Pointer
	// GCStateWorkOverflow is set when an object did not fit GCStateWork
	// and was left gcMarkPending.
	GCStateWorkOverflow bool
	// GCStateViews holds the pointers into an object, and those to an object
	// already queued with another type, that the mark has yet to follow.
	GCStateViews []GCVisit
	// GCStateTypes are the pointer and slice GCTypes, by GCTypeIndex-1.
	GCStateTypes []*GCType
}

// GCStateWorkSize is the entries of GCStateWork for a heap cap. A full
// GCStateWork leaves objects for a scan of the heap.
func GCStateWorkSize(heapCap types.Pointer) int {
	return max(256, int(heapCap/1024))
}

// GCStateViewsSize is the entries of GCStateViews for a memory size. More
// pointers into objects pending at once grow it, the one Go allocation a
// collection can make (D97).
func GCStateViewsSize(memorySize types.Pointer) int {
	return max(256, int(memorySize/512))
}

// GCStatePrepare allocates the scratch of state for a heap of at most
// heapCap bytes in a memory of at most memorySize bytes. It allocates
// nothing when state already fits them.
func GCStatePrepare(state *GCState, heapCap types.Pointer, memorySize types.Pointer) {
	if words := int(heapCap/64) + 1; len(state.GCStateStarts) < words {
		state.GCStateStarts = make([]uint64, words)
	}
	if words := int(memorySize/64) + 1; len(state.GCStateSlots) < words {
		state.GCStateSlots = make([]uint64, words)
		state.GCStateSlotWords = make([]uint64, words/64+1)
	}
	if size := GCStateWorkSize(heapCap); cap(state.GCStateWork) < size {
		state.GCStateWork = make([]types.Pointer, 0, size)
	}
	if size := GCStateViewsSize(memorySize); cap(state.GCStateViews) < size {
		state.GCStateViews = make([]GCVisit, 0, size)
	}
}

// GCStateTypeAdd gives the pointer or slice type t its GCTypeIndex.
func GCStateTypeAdd(state *GCState, t *GCType) {
	state.GCStateTypes = append(state.GCStateTypes, t)
	t.GCTypeIndex = types.Cast_int_to_ptr(len(state.GCStateTypes))
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
// the objects down. It returns the new heap pointer. state is prepared by
// GCStatePrepare for the heap and memory.
func GarbageCollectorCollect(memory []byte, heapStartsAt types.Pointer, heapPointer types.Pointer, roots *GCRootTable, state *GCState) types.Pointer {
	collection := gcCollection{memory: memory, heapStartsAt: heapStartsAt, heapPointer: heapPointer, state: state}
	collection.gcObjectStartsBuild()
	state.GCStateWork = state.GCStateWork[:0]
	state.GCStateViews = state.GCStateViews[:0]
	state.GCStateWorkOverflow = false
	state.GCStateSlotHigh = 0

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
	end := heapStartsAt + heapPointer
	for object := heapStartsAt + constants.NULL_HEAP_ADDRESS_OFFSET; object < end; {
		size := types.Read_obj_size(memory, object)
		if size == 0 {
			break
		}
		if memory[object] == gcMarkLive {
			types.Write_obj_forwarding_address(memory, object, heapStartsAt+forward)
			forward += size
		}
		object += size
	}

	collection.gcSlotsUpdate()

	for object := heapStartsAt + constants.NULL_HEAP_ADDRESS_OFFSET; object < end; {
		size := types.Read_obj_size(memory, object)
		if size == 0 {
			break
		}
		if memory[object] == gcMarkLive {
			memory[object] = 0
			to := types.Read_obj_forwarding_address(memory, object)
			copy(memory[to:to+size], memory[object:object+size])
		}
		object += size
	}
	clear(state.GCStateStarts[:heapPointer/64+1])
	return forward
}

// gcSlotsUpdate moves each reference the mark recorded, in address order,
// to the new address of the object it points into, and clears the record.
func (collection *gcCollection) gcSlotsUpdate() {
	state := collection.state
	memory := collection.memory
	for summary := 0; summary <= state.GCStateSlotHigh/64; summary++ {
		words := state.GCStateSlotWords[summary]
		state.GCStateSlotWords[summary] = 0
		for words != 0 {
			word := summary*64 + bits.TrailingZeros64(words)
			words &= words - 1
			slotBits := state.GCStateSlots[word]
			state.GCStateSlots[word] = 0
			for slotBits != 0 {
				slot := types.Cast_int_to_ptr(word*64 + bits.TrailingZeros64(slotBits))
				slotBits &= slotBits - 1
				pointer := types.Read_ptr(memory, slot)
				if object, found := collection.gcObjectContaining(pointer); found {
					types.Write_ptr(memory, slot, pointer-object+types.Read_obj_forwarding_address(memory, object))
				}
			}
		}
	}
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
// into, records slot for the update, and queues what the object holds. A
// slot read before is skipped: CX gives a memory location one type, so the
// first read queued all it refers to.
func (collection *gcCollection) gcMarkReference(slot types.Pointer, t *GCType) {
	if uint64(slot)+uint64(types.POINTER_SIZE) > uint64(len(collection.memory)) {
		return
	}
	pointer := types.Read_ptr(collection.memory, slot)
	object, found := collection.gcObjectContaining(pointer)
	if !found {
		return
	}
	state := collection.state
	word, bit := int(slot/64), slot%64
	if state.GCStateSlots[word]&(1<<bit) != 0 {
		return
	}
	state.GCStateSlots[word] |= 1 << bit
	state.GCStateSlotWords[word/64] |= 1 << (word % 64)
	state.GCStateSlotHigh = max(state.GCStateSlotHigh, word)
	if collection.memory[object] == 0 {
		collection.memory[object] = gcMarkLive
	}
	if t.GCTypeElem == nil {
		return
	}
	if pointer == object && t.GCTypeIndex != 0 {
		// The forwarding address field holds the type the object is queued
		// with until the mark ends.
		switch types.Read_obj_forwarding_address(collection.memory, object) {
		case 0:
			types.Write_obj_forwarding_address(collection.memory, object, t.GCTypeIndex)
			if len(state.GCStateWork) == cap(state.GCStateWork) {
				collection.memory[object] = gcMarkPending
				state.GCStateWorkOverflow = true
				return
			}
			state.GCStateWork = append(state.GCStateWork, object)
			return
		case t.GCTypeIndex:
			return
		}
	}
	// VIOLATION VMALLOC-011: grows GCStateViews past GCStateViewsSize, D97, #407.
	state.GCStateViews = append(state.GCStateViews, GCVisit{GCVisitAddress: pointer, GCVisitType: t})
}

// gcMarkDrain walks the queued objects and pointers until none is left,
// then scans the heap for objects left gcMarkPending, until none is.
func (collection *gcCollection) gcMarkDrain() {
	state := collection.state
	memory := collection.memory
	for {
		collection.gcMarkQueuesDrain()
		if !state.GCStateWorkOverflow {
			return
		}
		state.GCStateWorkOverflow = false
		end := collection.heapStartsAt + collection.heapPointer
		for object := collection.heapStartsAt + constants.NULL_HEAP_ADDRESS_OFFSET; object < end; {
			size := types.Read_obj_size(memory, object)
			if size == 0 {
				break
			}
			if memory[object] == gcMarkPending {
				memory[object] = gcMarkLive
				collection.gcVisit(object, state.GCStateTypes[types.Read_obj_forwarding_address(memory, object)-1])
				collection.gcMarkQueuesDrain()
			}
			object += size
		}
	}
}

// gcMarkQueuesDrain walks what GCStateViews and GCStateWork hold.
func (collection *gcCollection) gcMarkQueuesDrain() {
	state := collection.state
	for {
		if n := len(state.GCStateViews); n > 0 {
			visit := state.GCStateViews[n-1]
			state.GCStateViews = state.GCStateViews[:n-1]
			collection.gcVisit(visit.GCVisitAddress, visit.GCVisitType)
			continue
		}
		n := len(state.GCStateWork)
		if n == 0 {
			return
		}
		object := state.GCStateWork[n-1]
		state.GCStateWork = state.GCStateWork[:n-1]
		collection.gcVisit(object, state.GCStateTypes[types.Read_obj_forwarding_address(collection.memory, object)-1])
	}
}

// gcVisit marks what the pointer or slice of GCType t refers to. A pointer
// to the heap is the address of the value less the object header.
func (collection *gcCollection) gcVisit(pointer types.Pointer, t *GCType) {
	if t.GCTypeKind == GCKindPointer {
		collection.gcMarkValue(pointer+types.OBJECT_HEADER_SIZE, t.GCTypeElem)
		return
	}
	object, _ := collection.gcObjectContaining(pointer)
	elements := pointer + types.OBJECT_HEADER_SIZE + constants.SLICE_HEADER_SIZE
	end := object + types.Read_obj_size(collection.memory, object)
	if elements > end || t.GCTypeElemSize == 0 {
		return
	}
	length := types.Read_ptr(collection.memory, pointer+types.OBJECT_HEADER_SIZE+types.POINTER_SIZE)
	length = min(length, (end-elements)/t.GCTypeElemSize)
	for i := types.Pointer(0); i < length; i++ {
		collection.gcMarkValue(elements+i*t.GCTypeElemSize, t.GCTypeElem)
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

// gcObjectStartsBuild sets the bit of each heap object in GCStateStarts and
// clears its mark and forwarding address, which a new object may not.
func (collection *gcCollection) gcObjectStartsBuild() {
	starts := collection.state.GCStateStarts
	end := collection.heapStartsAt + collection.heapPointer
	for c := collection.heapStartsAt + constants.NULL_HEAP_ADDRESS_OFFSET; c < end; {
		offset := c - collection.heapStartsAt
		starts[offset/64] |= 1 << (offset % 64)
		collection.memory[c] = 0
		objSize := types.Read_obj_size(collection.memory, c)
		if objSize == 0 {
			break
		}
		types.Write_obj_forwarding_address(collection.memory, c, 0)
		c += objSize
	}
}

// gcObjectContaining returns the address of the heap object that pointer
// points into. A pointer inside an object, as &s[1] is, keeps it alive.
func (collection *gcCollection) gcObjectContaining(pointer types.Pointer) (types.Pointer, bool) {
	if pointer < collection.heapStartsAt+constants.NULL_HEAP_ADDRESS_OFFSET || pointer >= collection.heapStartsAt+collection.heapPointer {
		return 0, false
	}
	starts := collection.state.GCStateStarts
	offset := pointer - collection.heapStartsAt
	word := int(offset / 64)
	// The starts at or below offset in its word.
	startBits := starts[word] & (2<<(offset%64) - 1)
	for startBits == 0 {
		if word == 0 {
			return 0, false
		}
		word--
		startBits = starts[word]
	}
	start := collection.heapStartsAt + types.Cast_int_to_ptr(word*64+63-bits.LeadingZeros64(startBits))
	if pointer >= start+types.Read_obj_size(collection.memory, start) {
		return 0, false
	}
	return start, true
}
