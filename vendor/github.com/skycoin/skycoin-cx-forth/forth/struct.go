package forth

import "strconv"

// StructCellsSize is the default size of the struct space, in cells.
const StructCellsSize = 256

// The built-in struct types. They describe the struct model itself.
//
//	StructRef             ( Int64 struct type id, Int64 instance index )
//	StructTypeDescriptor  ( Int64 ordinal count, StructRef first StructOrdinal )
//	StructOrdinal         ( Int64 atomic type, StructRef next StructOrdinal or nil )
//
// The struct table is a list of StructTypeDescriptor instances, each with
// its chain of StructOrdinal instances, all in the struct space.
const (
	StructTypeIDStructRef int64 = iota
	StructTypeIDStructTypeDescriptor
	StructTypeIDStructOrdinal
	StructTypeIDBuiltInCount
)

// structTypeEntry indexes one struct type's descriptor in the struct space.
// The ordinals of a type are laid out one after another, so ordinal k is at
// ordinals + 2*k.
type structTypeEntry struct {
	descriptor, ordinals, count int32
}

// StructRefMake returns the struct ref of the instance of struct type id at
// index in the struct space.
func StructRefMake(id int64, index int32) Cell {
	return Cell{AtomicTypeStructRef, id<<32 | int64(uint32(index))}
}

func structRefType(c Cell) int64    { return c.V >> 32 }
func structRefIndex(c Cell) int32   { return int32(uint32(c.V)) }
func structRefNil(c Cell) bool      { return c.V == 0 }
func (vm *VM) structSpaceInit()     { vm.structTop = 1 } // index 0 is the nil ref
func (vm *VM) StructTypeCount() int { return len(vm.structTypes) }

// structAllocate reserves n cells of the struct space and returns their index.
func (vm *VM) structAllocate(n int) int32 {
	if vm.structTop+n > len(vm.structCells) {
		vm.fail("struct space full")
		return -1
	}
	i := vm.structTop
	vm.structTop += n
	return int32(i)
}

// structTypeDefine adds a struct type with the given ordinal types and
// returns its id, or -1 on failure.
func (vm *VM) structTypeDefine(fields []AtomicType) int64 {
	if len(fields) == 0 {
		vm.fail("struct with no fields")
		return -1
	}
	if len(vm.structTypes) >= 1<<31-1 {
		vm.fail("struct table full")
		return -1
	}
	ordinals := vm.structAllocate(2 * len(fields))
	descriptor := vm.structAllocate(2)
	if vm.err != nil {
		if ordinals > 0 {
			vm.structTop = int(ordinals)
		}
		return -1
	}
	for k, t := range fields {
		i := ordinals + int32(2*k)
		vm.structCells[i] = Int(int64(t))
		vm.structCells[i+1] = Cell{AtomicTypeStructRef, 0}
		if k+1 < len(fields) {
			vm.structCells[i+1] = StructRefMake(StructTypeIDStructOrdinal, i+2)
		}
	}
	vm.structCells[descriptor] = Int(int64(len(fields)))
	vm.structCells[descriptor+1] = StructRefMake(StructTypeIDStructOrdinal, ordinals)
	vm.structTypes = append(vm.structTypes, structTypeEntry{descriptor, ordinals, int32(len(fields))})
	return int64(len(vm.structTypes) - 1)
}

func (vm *VM) structTypeBuiltInDefine() {
	vm.structSpaceInit()
	two := func(a, b AtomicType) []AtomicType { return []AtomicType{a, b} }
	vm.structTypeDefine(two(AtomicTypeInt64, AtomicTypeInt64))
	vm.structTypeDefine(two(AtomicTypeInt64, AtomicTypeStructRef))
	vm.structTypeDefine(two(AtomicTypeInt64, AtomicTypeStructRef))
}

// structTypeEntryOf returns the table entry of struct type id.
func (vm *VM) structTypeEntryOf(id int64) (structTypeEntry, bool) {
	if id < 0 || id >= int64(len(vm.structTypes)) {
		vm.fail("bad struct type id " + strconv.FormatInt(id, 10))
		return structTypeEntry{}, false
	}
	return vm.structTypes[id], true
}

// StructTypeOrdinalTypes returns the ordinal types of struct type id, read
// from its descriptor in the struct space.
func (vm *VM) StructTypeOrdinalTypes(id int64) []AtomicType {
	e, ok := vm.structTypeEntryOf(id)
	if !ok {
		return nil
	}
	ts := make([]AtomicType, e.count)
	for k := range ts {
		ts[k] = AtomicType(vm.structCells[e.ordinals+int32(2*k)].V)
	}
	return ts
}

// structOrdinalType returns the type of ordinal k of struct type id.
func (vm *VM) structOrdinalType(id int64, k int32) AtomicType {
	return AtomicType(vm.structCells[vm.structTypes[id].ordinals+2*k].V)
}

// structInstanceNew allocates an instance of struct type id with every
// field at the zero of its type.
func (vm *VM) structInstanceNew(id int64) Cell {
	e, ok := vm.structTypeEntryOf(id)
	if !ok {
		return Cell{}
	}
	i := vm.structAllocate(int(e.count))
	if vm.err != nil {
		return Cell{}
	}
	for k := int32(0); k < e.count; k++ {
		vm.structCells[i+k] = vm.atomicTypeZero(vm.structOrdinalType(id, k))
	}
	return StructRefMake(id, i)
}

// structRefCheck fails unless c is a non-nil ref whose instance fits in the
// struct space, and returns its type entry.
func (vm *VM) structRefCheck(word string, c Cell) (structTypeEntry, bool) {
	if !vm.atomicTypeWant(word, AtomicTypeStructRef, c) {
		return structTypeEntry{}, false
	}
	if structRefNil(c) {
		vm.fail(word + ": nil struct ref")
		return structTypeEntry{}, false
	}
	e, ok := vm.structTypeEntryOf(structRefType(c))
	if !ok {
		return e, false
	}
	if i := structRefIndex(c); i < 1 || int(i)+int(e.count) > vm.structTop {
		vm.fail(word + ": bad struct ref")
		return e, false
	}
	return e, true
}

func (vm *VM) popStructTypeID() (int64, bool) {
	c := vm.Pop()
	if vm.err != nil {
		return 0, false
	}
	if c.T != AtomicTypeInt64 {
		vm.fail("bad struct type id")
		return 0, false
	}
	_, ok := vm.structTypeEntryOf(c.V)
	return c.V, ok
}

func structPrims() []prim {
	builtIn := func(id int64) func(*VM) { return func(vm *VM) { vm.Push(Int(id)) } }
	return []prim{
		// StructDefineStart ( -- )
		{"StructDefineStart", func(vm *VM) {
			if vm.structDefineOpen {
				vm.fail("StructDefineStart: a struct is already being defined")
				return
			}
			vm.structDefineOpen = true
			vm.structDefineFields = vm.structDefineFields[:0]
		}, false},
		// StructDefineFieldAdd ( t -- ) adds the next ordinal.
		{"StructDefineFieldAdd", func(vm *VM) {
			t, ok := vm.popAtomicType()
			switch {
			case !ok:
			case !vm.structDefineOpen:
				vm.fail("StructDefineFieldAdd without StructDefineStart")
			default:
				vm.structDefineFields = append(vm.structDefineFields, t)
			}
		}, false},
		// StructDefineEnd ( -- id ) submits the struct type.
		{"StructDefineEnd", func(vm *VM) {
			if !vm.structDefineOpen {
				vm.fail("StructDefineEnd without StructDefineStart")
				return
			}
			vm.structDefineOpen = false
			if id := vm.structTypeDefine(vm.structDefineFields); id >= 0 {
				vm.Push(Int(id))
			}
		}, false},
		{"StructTypeIDStructRef", builtIn(StructTypeIDStructRef), false},
		{"StructTypeIDStructTypeDescriptor", builtIn(StructTypeIDStructTypeDescriptor), false},
		{"StructTypeIDStructOrdinal", builtIn(StructTypeIDStructOrdinal), false},
		// StructTypeCount ( -- n )
		{"StructTypeCount", func(vm *VM) { vm.Push(Int(int64(len(vm.structTypes)))) }, false},
		// StructTypeDescriptorRef ( id -- ref )
		{"StructTypeDescriptorRef", func(vm *VM) {
			if id, ok := vm.popStructTypeID(); ok {
				vm.Push(StructRefMake(StructTypeIDStructTypeDescriptor, vm.structTypes[id].descriptor))
			}
		}, false},
		// StructInstanceNew ( id -- ref )
		{"StructInstanceNew", func(vm *VM) {
			if id, ok := vm.popStructTypeID(); ok {
				if c := vm.structInstanceNew(id); vm.err == nil {
					vm.Push(c)
				}
			}
		}, false},
		// StructRefTypeID ( ref -- id )
		{"StructRefTypeID", func(vm *VM) {
			c := vm.Pop()
			if vm.atomicTypeWant("StructRefTypeID", AtomicTypeStructRef, c) {
				vm.Push(Int(structRefType(c)))
			}
		}, false},
	}
}
