package opcodes

import (
	"math/rand"
	"strconv"

	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates strs, which
	// adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// The built-in str function returns the base 10 string representation of operand 1.
func OpUI32ToStr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	formatted := strconv.AppendUint(formatBuffer(prgrm), uint64(inputs[0].Get_ui32(prgrm)), 10)
	outputs[0].Set_str_bytes(prgrm, formatted)
	formatBufferKeep(prgrm, formatted)
}

// The built-in i8 function returns operand 1 casted from type ui32 to type i8.
func OpUI32ToI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int8(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_i8(prgrm, outV0)
}

// The built-in i16 function returns operand 1 casted from type ui32 to type i16.
func OpUI32ToI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int16(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_i16(prgrm, outV0)
}

// The built-in i32 function return operand 1 casted from type ui32 to type i32.
func OpUI32ToI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int32(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in i64 function returns operand 1 casted from type ui32 to type i64.
func OpUI32ToI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int64(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_i64(prgrm, outV0)
}

// The built-in ui8 function returns operand 1 casted from type ui32 to type ui8.
func OpUI32ToUI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := uint8(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_ui8(prgrm, outV0)
}

// The built-in ui16 function returns the operand 1 casted from type ui32 to type ui16.
func OpUI32ToUI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := uint16(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_ui16(prgrm, outV0)
}

// The built-in ui64 function returns the operand 1 casted from type ui32 to type ui64.
func OpUI32ToUI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := uint64(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_ui64(prgrm, outV0)
}

// The built-in f32 function returns operand 1 casted from type ui32 to type f32.
func OpUI32ToF32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := float32(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_f32(prgrm, outV0)
}

// The built-in f64 function returns operand 1 casted from type ui32 to type f64.
func OpUI32ToF64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := float64(inputs[0].Get_ui32(prgrm))
	outputs[0].Set_f64(prgrm, outV0)
}

// The print built-in function formats its arguments and prints them.
func OpUI32Print(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	printFormattedLine(prgrm, strconv.AppendUint(formatBuffer(prgrm), uint64(inputs[0].Get_ui32(prgrm)), 10))
}

// The built-in add function returns the sum of two ui32 numbers.
func OpUI32Add(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) + inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in sub function returns the difference of two ui32 numbers.
func OpUI32Sub(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) - inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in mul function returns the product of two ui32 numbers.
func OpUI32Mul(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) * inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in div function returns the quotient of two ui32 numbers.
func OpUI32Div(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) / inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in gt function returns true if operand 1 is greater than operand 2.
func OpUI32Gt(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) > inputs[1].Get_ui32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in gteq function returns true if operand 1 is greater than or
// equal to operand 2.
func OpUI32Gteq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) >= inputs[1].Get_ui32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in lt function returns true if operand 1 is less than operand 2.
func OpUI32Lt(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) < inputs[1].Get_ui32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in lteq function returns true if operand 1 is less than or equal
// to operand 1.
func OpUI32Lteq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) <= inputs[1].Get_ui32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in eq function returns true if operand 1 is equal to operand 2.
func OpUI32Eq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) == inputs[1].Get_ui32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in uneq function returns true if operand 1 is different from operand 2.
func OpUI32Uneq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) != inputs[1].Get_ui32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in mod function returns the remainder of operand 1 / operand 2.
func OpUI32Mod(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) % inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in rand function returns a pseudo-random number.
func OpUI32Rand(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := rand.Uint32()
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in bitand function returns the bitwise AND of 2 operands.
func OpUI32Bitand(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) & inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in bitor function returns the bitwise OR of 2 operands.
func OpUI32Bitor(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) | inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in bitxor function returns the bitwise XOR of 2 operands.
func OpUI32Bitxor(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) ^ inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in bitclear function returns the bitwise AND NOT of 2 operands.
func OpUI32Bitclear(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) &^ inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in bitshl function returns bits of operand 1 shifted to the left
// by number of positions specified in operand 2.
func OpUI32Bitshl(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) << inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in bitshr function returns bits of operand 1 shifted to the right
// by number of positions specified in operand 2.
func OpUI32Bitshr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_ui32(prgrm) >> inputs[1].Get_ui32(prgrm)
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in max function returns the biggest of the two operands.
func OpUI32Max(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_ui32(prgrm)
	inpV1 := inputs[1].Get_ui32(prgrm)
	if inpV1 > inpV0 {
		inpV0 = inpV1
	}
	outputs[0].Set_ui32(prgrm, inpV0)
}

// The built-in min function returns the smallest of the two operands.
func OpUI32Min(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_ui32(prgrm)
	inpV1 := inputs[1].Get_ui32(prgrm)
	if inpV1 < inpV0 {
		inpV0 = inpV1
	}
	outputs[0].Set_ui32(prgrm, inpV0)
}
