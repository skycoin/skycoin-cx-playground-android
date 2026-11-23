package forth

import "strconv"

// The registers. One of them is the focus, and the Register words act on
// it. The registers and the data stack are independent.
const (
	RegisterLeft = iota
	RegisterMiddle
	RegisterRight
	RegisterCount
)

// A register is a cell. Its tag is the register's declared AtomicType,
// which RegisterTypeSet changes and RegisterFocusPopFromStack checks. A new
// register holds the Int64 0.

// RegisterFocus returns the index of the focus register.
func (vm *VM) RegisterFocus() int { return int(vm.registerFocus) }

// RegisterValue returns the content of register i.
func (vm *VM) RegisterValue(i int) Cell { return vm.registers[i] }

func (vm *VM) registerFocusCell() *Cell { return &vm.registers[vm.registerFocus] }

// registerFocusOperate runs an arithmetic word on the focus and an operand
// of the focus type, and keeps the result in the focus.
func (vm *VM) registerFocusOperate(word string, op byte) {
	x := vm.Pop()
	focus := vm.registerFocusCell()
	if !vm.atomicTypeWant(word, focus.T, x) {
		return
	}
	if !atomicTypeNumeric(focus.T) && !(op == '+' && focus.T == AtomicTypeString) {
		vm.fail("type error: " + word + " on " + AtomicTypeName(focus.T))
		return
	}
	vm.Push(*focus)
	vm.Push(x)
	vm.arith(op)
	if vm.err == nil {
		*focus = vm.Pop()
	}
}

func registerPrims() []prim {
	focus := func(i uint8) func(*VM) { return func(vm *VM) { vm.registerFocus = i } }
	return []prim{
		// RegisterFocusLeft RegisterFocusMiddle RegisterFocusRight ( -- )
		{"RegisterFocusLeft", focus(RegisterLeft), false},
		{"RegisterFocusMiddle", focus(RegisterMiddle), false},
		{"RegisterFocusRight", focus(RegisterRight), false},
		// RegisterFocusPushToStack ( -- x ) copies the focus to the stack.
		{"RegisterFocusPushToStack", func(vm *VM) { vm.Push(*vm.registerFocusCell()) }, false},
		// RegisterFocusPopFromStack ( x -- ) moves x into the focus. x must
		// have the focus type.
		{"RegisterFocusPopFromStack", func(vm *VM) {
			x := vm.Pop()
			if f := vm.registerFocusCell(); vm.atomicTypeWant("RegisterFocusPopFromStack", f.T, x) {
				*f = x
			}
		}, false},
		{"RegisterFocusAdd", func(vm *VM) { vm.registerFocusOperate("RegisterFocusAdd", '+') }, false},
		{"RegisterFocusSubtract", func(vm *VM) { vm.registerFocusOperate("RegisterFocusSubtract", '-') }, false},
		{"RegisterFocusMultiply", func(vm *VM) { vm.registerFocusOperate("RegisterFocusMultiply", '*') }, false},
		{"RegisterFocusDivide", func(vm *VM) { vm.registerFocusOperate("RegisterFocusDivide", '/') }, false},
		{"RegisterFocusModulo", func(vm *VM) { vm.registerFocusOperate("RegisterFocusModulo", '%') }, false},
		// RegisterFocusNegate ( -- )
		{"RegisterFocusNegate", func(vm *VM) {
			f := vm.registerFocusCell()
			if !atomicTypeNumeric(f.T) {
				vm.fail("type error: RegisterFocusNegate on " + AtomicTypeName(f.T))
				return
			}
			vm.Push(Cell{f.T, 0})
			vm.Push(*f)
			vm.arith('-')
			if vm.err == nil {
				*f = vm.Pop()
			}
		}, false},
		// RegisterClear ( -- ) sets the focus to the zero of its type.
		{"RegisterClear", func(vm *VM) { f := vm.registerFocusCell(); *f = vm.atomicTypeZero(f.T) }, false},
		// RegisterTypeSet ( t -- ) gives the focus type t and its zero.
		{"RegisterTypeSet", func(vm *VM) {
			if t, ok := vm.popAtomicType(); ok {
				*vm.registerFocusCell() = vm.atomicTypeZero(t)
			}
		}, false},
		// RegisterTypeGet ( -- t )
		{"RegisterTypeGet", func(vm *VM) { vm.Push(Int(int64(vm.registerFocusCell().T))) }, false},
	}
}

// appendStructRef formats a struct ref as StructRef(type id, index).
func (vm *VM) appendStructRef(b []byte, c Cell) []byte {
	if c.V == 0 {
		return append(b, "StructRef(nil)"...)
	}
	b = append(b, "StructRef("...)
	b = strconv.AppendInt(b, c.V>>32, 10)
	b = append(b, ',')
	b = strconv.AppendInt(b, c.V&0xffffffff, 10)
	return append(b, ')')
}
