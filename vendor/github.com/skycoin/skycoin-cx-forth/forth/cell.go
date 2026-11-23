package forth

import (
	"math"
	"strconv"
)

// Ternary values in balanced form, so Kleene AND is min, OR is max
// and NOT is negation.
const (
	False int64 = -1
	Null  int64 = 0
	True  int64 = 1
)

// Cell is one stack or memory slot.
type Cell struct {
	T AtomicType
	V int64
}

// Int makes an integer cell.
func Int(v int64) Cell { return Cell{TInt, v} }

// F64 makes a float64 cell.
func F64(f float64) Cell { return Cell{TF64, int64(math.Float64bits(f))} }

// F32 makes a float32 cell.
func F32(f float32) Cell { return Cell{TF32, int64(math.Float64bits(float64(f)))} }

// Tri makes a ternary cell from False, Null or True.
func Tri(v int64) Cell { return Cell{TTri, v} }

// Bool makes a ternary cell that is True or False.
func Bool(b bool) Cell {
	if b {
		return Tri(True)
	}
	return Tri(False)
}

// Float returns the cell as a float64, converting integers.
func (c Cell) Float() float64 {
	switch c.T {
	case TF64, TF32:
		return math.Float64frombits(uint64(c.V))
	case AtomicTypeUint64:
		return float64(uint64(c.V))
	}
	return float64(c.V)
}

// IsFloat reports whether c holds a float.
func (c Cell) IsFloat() bool { return c.T == TF64 || c.T == TF32 }

// IsNull reports whether c is the ternary null.
func (c Cell) IsNull() bool { return c.T == TTri && c.V == Null }

// Truth maps a cell to a ternary value. Integers follow Forth flags,
// zero is false and anything else is true.
func (c Cell) Truth() int64 {
	switch c.T {
	case TTri:
		return c.V
	case TF64, TF32:
		if c.Float() == 0 {
			return False
		}
		return True
	}
	if c.V == 0 {
		return False
	}
	return True
}

// TriName returns the printed name of a ternary value.
func TriName(v int64) string {
	switch v {
	case True:
		return "true"
	case False:
		return "false"
	}
	return "null"
}

// appendCell formats c without going through fmt.
func (vm *VM) appendCell(b []byte, c Cell) []byte {
	switch c.T {
	case TF64:
		return strconv.AppendFloat(b, c.Float(), 'g', -1, 64)
	case TF32:
		return strconv.AppendFloat(b, c.Float(), 'g', -1, 32)
	case TTri:
		return append(b, TriName(c.V)...)
	case TStr:
		return append(b, vm.str(c)...)
	case AtomicTypeUint64:
		return strconv.AppendUint(b, uint64(c.V), vm.base)
	case AtomicTypeBool:
		return strconv.AppendBool(b, c.V != 0)
	case AtomicTypeStructRef:
		return vm.appendStructRef(b, c)
	}
	return strconv.AppendInt(b, c.V, vm.base)
}
