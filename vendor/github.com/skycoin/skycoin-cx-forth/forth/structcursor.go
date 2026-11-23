package forth

// The struct cursor walks the ordinals of one struct instance. It is VM
// state beside the registers and the stack. StructCursorNext at the last
// ordinal and StructCursorPrevious at ordinal 0 stay put and push FALSE.

// structCursorField returns the struct space index of the field under the
// cursor, or -1 when no struct is open.
func (vm *VM) structCursorField(word string) int32 {
	if structRefNil(vm.structCursorRef) {
		vm.fail(word + ": no struct open")
		return -1
	}
	return structRefIndex(vm.structCursorRef) + vm.structCursorOrdinal
}

func (vm *VM) structCursorCount() int32 {
	return vm.structTypes[structRefType(vm.structCursorRef)].count
}

// structCursorMove moves the cursor by step and pushes TRUE, or stays and
// pushes FALSE when that would leave the struct.
func (vm *VM) structCursorMove(word string, step int32) {
	if vm.structCursorField(word) < 0 {
		return
	}
	k := vm.structCursorOrdinal + step
	if k < 0 || k >= vm.structCursorCount() {
		vm.Push(Tri(False))
		return
	}
	vm.structCursorOrdinal = k
	vm.Push(Tri(True))
}

// structCursorClose closes the cursor if its instance is at or above index,
// which is about to be freed.
func (vm *VM) structCursorClose(index int) {
	if !structRefNil(vm.structCursorRef) && int(structRefIndex(vm.structCursorRef)) >= index {
		vm.structCursorRef = Cell{}
	}
}

// structSpaceFloor is the lowest index StructSpaceRelease may free to: the
// end of the last struct type's descriptor.
func (vm *VM) structSpaceFloor() int {
	return int(vm.structTypes[len(vm.structTypes)-1].descriptor) + 2
}

func structCursorPrims() []prim {
	return []prim{
		// StructCursorOpen ( ref -- ) puts the cursor at ordinal 0 of ref.
		{"StructCursorOpen", func(vm *VM) {
			c := vm.Pop()
			if _, ok := vm.structRefCheck("StructCursorOpen", c); ok {
				vm.structCursorRef, vm.structCursorOrdinal = c, 0
			}
		}, false},
		// StructCursorFirst StructCursorLast ( -- )
		{"StructCursorFirst", func(vm *VM) {
			if vm.structCursorField("StructCursorFirst") >= 0 {
				vm.structCursorOrdinal = 0
			}
		}, false},
		{"StructCursorLast", func(vm *VM) {
			if vm.structCursorField("StructCursorLast") >= 0 {
				vm.structCursorOrdinal = vm.structCursorCount() - 1
			}
		}, false},
		// StructCursorNext StructCursorPrevious ( -- moved )
		{"StructCursorNext", func(vm *VM) { vm.structCursorMove("StructCursorNext", 1) }, false},
		{"StructCursorPrevious", func(vm *VM) { vm.structCursorMove("StructCursorPrevious", -1) }, false},
		// StructCursorOrdinal ( -- n )
		{"StructCursorOrdinal", func(vm *VM) {
			if vm.structCursorField("StructCursorOrdinal") >= 0 {
				vm.Push(Int(int64(vm.structCursorOrdinal)))
			}
		}, false},
		// StructCursorOrdinalSet ( n -- ) moves the cursor to ordinal n.
		{"StructCursorOrdinalSet", func(vm *VM) {
			n := vm.Pop()
			if vm.err != nil || vm.structCursorField("StructCursorOrdinalSet") < 0 {
				return
			}
			if n.T != AtomicTypeInt64 || n.V < 0 || n.V >= int64(vm.structCursorCount()) {
				vm.fail("StructCursorOrdinalSet: no such ordinal")
				return
			}
			vm.structCursorOrdinal = int32(n.V)
		}, false},
		// StructCursorOrdinalType ( -- t )
		{"StructCursorOrdinalType", func(vm *VM) {
			if vm.structCursorField("StructCursorOrdinalType") >= 0 {
				vm.Push(Int(int64(vm.structOrdinalType(structRefType(vm.structCursorRef), vm.structCursorOrdinal))))
			}
		}, false},
		// StructCursorFieldGet ( -- x )
		{"StructCursorFieldGet", func(vm *VM) {
			if i := vm.structCursorField("StructCursorFieldGet"); i >= 0 {
				vm.Push(vm.structCells[i])
			}
		}, false},
		// StructCursorFieldSet ( x -- ) x must have the ordinal's type.
		{"StructCursorFieldSet", func(vm *VM) {
			x := vm.Pop()
			if i := vm.structCursorField("StructCursorFieldSet"); i >= 0 &&
				vm.atomicTypeWant("StructCursorFieldSet", vm.structOrdinalType(structRefType(vm.structCursorRef), vm.structCursorOrdinal), x) {
				vm.structCells[i] = x
			}
		}, false},
		// StructUnpack ( ref -- x0 ... xn-1 ) pushes the fields in ordinal order.
		{"StructUnpack", func(vm *VM) {
			c := vm.Pop()
			if e, ok := vm.structRefCheck("StructUnpack", c); ok {
				for k := int32(0); k < e.count; k++ {
					vm.Push(vm.structCells[structRefIndex(c)+k])
				}
			}
		}, false},
		// StructSpaceTop ( -- n ) and StructSpaceRelease ( n -- ) free the
		// instances made since StructSpaceTop returned n.
		{"StructSpaceTop", func(vm *VM) { vm.Push(Int(int64(vm.structTop))) }, false},
		{"StructSpaceRelease", func(vm *VM) {
			n := vm.Pop()
			if vm.err != nil {
				return
			}
			if n.T != AtomicTypeInt64 || n.V < int64(vm.structSpaceFloor()) || n.V > int64(vm.structTop) {
				vm.fail("StructSpaceRelease: bad struct space index")
				return
			}
			vm.structCursorClose(int(n.V))
			vm.structTop = int(n.V)
		}, false},
	}
}
