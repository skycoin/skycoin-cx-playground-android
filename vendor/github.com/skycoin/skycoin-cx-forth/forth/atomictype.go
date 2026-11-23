package forth

import "strconv"

// AtomicType is the type of a cell, of a register and of a struct field.
// The set is finite. The first five keep the values the cell tags had.
type AtomicType uint8

// The atomic types.
const (
	AtomicTypeInt64     AtomicType = iota // also addresses and Forth flags
	AtomicTypeFloat64                     // V holds the float64 bits
	AtomicTypeFloat32                     // V holds the value widened to float64 bits
	AtomicTypeTernary                     // V is one of False, Null, True
	AtomicTypeString                      // V indexes the VM string pool
	AtomicTypeInt8                        // the integer types hold V wrapped to their width
	AtomicTypeInt16                       //
	AtomicTypeInt32                       //
	AtomicTypeUint8                       //
	AtomicTypeUint16                      //
	AtomicTypeUint32                      //
	AtomicTypeUint64                      // V holds the bits
	AtomicTypeBool                        // CX bool, V is 0 or 1
	AtomicTypeStructRef                   // V is struct type id << 32 | instance index, 0 is the nil ref
	AtomicTypeCount                       // the number of atomic types, not a type
)

// Tag is the old name of AtomicType. It goes when the short names do.
type Tag = AtomicType

// The old names of the first five atomic types.
const (
	TInt = AtomicTypeInt64
	TF64 = AtomicTypeFloat64
	TF32 = AtomicTypeFloat32
	TTri = AtomicTypeTernary
	TStr = AtomicTypeString
)

var atomicTypeNames = [AtomicTypeCount]string{
	"Int64", "Float64", "Float32", "Ternary", "String",
	"Int8", "Int16", "Int32", "Uint8", "Uint16", "Uint32", "Uint64",
	"Bool", "StructRef",
}

// AtomicTypeName returns the name of t without the AtomicType prefix, such
// as Int32. The word that pushes t is AtomicType followed by that name.
func AtomicTypeName(t AtomicType) string {
	if t < AtomicTypeCount {
		return atomicTypeNames[t]
	}
	return "AtomicType" + strconv.Itoa(int(t))
}

func atomicTypeInteger(t AtomicType) bool {
	return t == AtomicTypeInt64 || (t >= AtomicTypeInt8 && t <= AtomicTypeUint64)
}

func atomicTypeUnsigned(t AtomicType) bool { return t >= AtomicTypeUint8 && t <= AtomicTypeUint64 }

func atomicTypeFloat(t AtomicType) bool { return t == AtomicTypeFloat64 || t == AtomicTypeFloat32 }

func atomicTypeNumeric(t AtomicType) bool { return atomicTypeInteger(t) || atomicTypeFloat(t) }

// atomicTypeWrap makes an integer cell of type t from v, wrapped to t's width.
func atomicTypeWrap(t AtomicType, v int64) Cell {
	switch t {
	case AtomicTypeInt8:
		v = int64(int8(v))
	case AtomicTypeInt16:
		v = int64(int16(v))
	case AtomicTypeInt32:
		v = int64(int32(v))
	case AtomicTypeUint8:
		v = int64(uint8(v))
	case AtomicTypeUint16:
		v = int64(uint16(v))
	case AtomicTypeUint32:
		v = int64(uint32(v))
	}
	return Cell{t, v}
}

// atomicTypeIntegerResult is the type of integer arithmetic on a and b. An
// Int64 operand, which is what a literal is, takes the other operand's type.
func atomicTypeIntegerResult(a, b AtomicType) (AtomicType, bool) {
	switch {
	case a == b:
		return a, true
	case a == AtomicTypeInt64:
		return b, true
	case b == AtomicTypeInt64:
		return a, true
	}
	return 0, false
}

// atomicTypeCompareInteger compares two integer cells. Only Uint64 can
// hold a value outside int64.
func atomicTypeCompareInteger(a, b Cell) int {
	au, bu := a.T == AtomicTypeUint64, b.T == AtomicTypeUint64
	switch {
	case au && bu:
		return compareOrdered(uint64(a.V), uint64(b.V))
	case au && a.V < 0:
		return 1
	case bu && b.V < 0:
		return -1
	}
	return compareOrdered(a.V, b.V)
}

func compareOrdered[T int64 | uint64 | float64 | string](x, y T) int {
	switch {
	case x < y:
		return -1
	case x > y:
		return 1
	}
	return 0
}

// atomicTypeZero returns the zero value of t: 0, 0.0, NULL, "", false or the
// nil struct ref.
func (vm *VM) atomicTypeZero(t AtomicType) Cell {
	if t == AtomicTypeString {
		return vm.intern("")
	}
	return Cell{t, 0}
}

// atomicTypeConvert returns c as type t. NULL passes through, as it does
// for >I32. A struct ref converts only to itself.
func (vm *VM) atomicTypeConvert(c Cell, t AtomicType) (Cell, bool) {
	switch {
	case t >= AtomicTypeCount:
		vm.fail("bad atomic type " + strconv.Itoa(int(t)))
		return Cell{}, false
	case c.T == t:
		return c, true
	case c.IsNull():
		return c, true
	case t == AtomicTypeString:
		return vm.intern(string(vm.appendCell(nil, c))), true
	case t == AtomicTypeTernary:
		if c.T == AtomicTypeString || c.T == AtomicTypeStructRef {
			break
		}
		return Tri(c.Truth()), true
	case t == AtomicTypeBool:
		if c.T == AtomicTypeString || c.T == AtomicTypeStructRef {
			break
		}
		return Boolean(c.Truth() == True), true
	case !atomicTypeNumeric(t) || !(atomicTypeNumeric(c.T) || c.T == AtomicTypeBool):
	case t == AtomicTypeFloat64:
		return F64(c.Float()), true
	case t == AtomicTypeFloat32:
		return F32(float32(c.Float())), true
	case atomicTypeFloat(c.T) && atomicTypeUnsigned(t):
		return atomicTypeWrap(t, int64(uint64(c.Float()))), true
	case atomicTypeFloat(c.T):
		return atomicTypeWrap(t, int64(c.Float())), true
	default:
		return atomicTypeWrap(t, c.V), true
	}
	vm.fail("type error: cannot convert " + AtomicTypeName(c.T) + " to " + AtomicTypeName(t))
	return Cell{}, false
}

// atomicTypeWant fails unless c has type t. It is the run time assert
// behind the typed words.
func (vm *VM) atomicTypeWant(word string, t AtomicType, c Cell) bool {
	if vm.err != nil {
		return false
	}
	if c.T != t {
		vm.fail("type error: " + word + " wants " + AtomicTypeName(t) + ", got " + AtomicTypeName(c.T))
		return false
	}
	return true
}

// popAtomicType pops a type id that one of the AtomicType words pushed.
func (vm *VM) popAtomicType() (AtomicType, bool) {
	c := vm.Pop()
	if vm.err != nil {
		return 0, false
	}
	if c.T != AtomicTypeInt64 || c.V < 0 || c.V >= int64(AtomicTypeCount) {
		vm.fail("bad atomic type")
		return 0, false
	}
	return AtomicType(c.V), true
}

func atomicTypePrims() []prim {
	ps := make([]prim, 0, AtomicTypeCount+3)
	for t := AtomicType(0); t < AtomicTypeCount; t++ {
		id := Int(int64(t))
		ps = append(ps, prim{"AtomicType" + atomicTypeNames[t], func(vm *VM) { vm.Push(id) }, false})
	}
	return append(ps,
		// AtomicTypeConvert ( x t -- y )
		prim{"AtomicTypeConvert", func(vm *VM) {
			if t, ok := vm.popAtomicType(); ok {
				if c, ok := vm.atomicTypeConvert(vm.Pop(), t); ok {
					vm.Push(c)
				}
			}
		}, false},
		// AtomicTypeOf ( x -- t )
		prim{"AtomicTypeOf", func(vm *VM) { vm.Push(Int(int64(vm.Pop().T))) }, false},
		// AtomicTypeNameOf ( t -- s )
		prim{"AtomicTypeNameOf", func(vm *VM) {
			if t, ok := vm.popAtomicType(); ok {
				vm.Push(vm.intern(atomicTypeNames[t]))
			}
		}, false},
	)
}

// Boolean makes an AtomicTypeBool cell, the CX bool rather than a ternary.
func Boolean(b bool) Cell {
	if b {
		return Cell{AtomicTypeBool, 1}
	}
	return Cell{AtomicTypeBool, 0}
}

func atomicTypeFloatCell(t AtomicType, f float64) Cell {
	if t == AtomicTypeFloat32 {
		return F32(float32(f))
	}
	return F64(f)
}
