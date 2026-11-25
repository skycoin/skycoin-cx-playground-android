package opcodes

import (
	"math/rand"
	"strconv"

	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates strs, which
	// adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// The built-in str function returns the base 10 string representation of operand 1.
func OpI32ToStr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	formatted := strconv.AppendInt(formatBuffer(prgrm), int64(inputs[0].Get_i32(prgrm)), 10)
	outputs[0].Set_str_bytes(prgrm, formatted)
	formatBufferKeep(prgrm, formatted)
}

// The built-in i8 function returns operand 1 casted from type i32 to type i8.
func OpI32ToI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int8(inputs[0].Get_i32(prgrm))
	outputs[0].Set_i8(prgrm, outV0)
}

// The built-in i16 function returns operand 1 casted from type i32 to type i16.
func OpI32ToI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int16(inputs[0].Get_i32(prgrm))
	outputs[0].Set_i16(prgrm, outV0)
}

// The built-in i64 function returns operand 1 casted from type i32 to type i64.
func OpI32ToI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := int64(inputs[0].Get_i32(prgrm))
	outputs[0].Set_i64(prgrm, outV0)
}

// The built-in ui8 function returns operand 1 casted from type i32 to type ui8.
func OpI32ToUI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := uint8(inputs[0].Get_i32(prgrm))
	outputs[0].Set_ui8(prgrm, outV0)
}

// The built-in ui16 function returns the operand 1 casted from type i32 to type ui16.
func OpI32ToUI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := uint16(inputs[0].Get_i32(prgrm))
	outputs[0].Set_ui16(prgrm, outV0)
}

// The built-in ui32 function returns the operand 1 casted from type i32 to type ui32.
func OpI32ToUI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := uint32(inputs[0].Get_i32(prgrm))
	outputs[0].Set_ui32(prgrm, outV0)
}

// The built-in ui64 function returns the operand 1 casted from type i32 to type ui64.
func OpI32ToUI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := uint64(inputs[0].Get_i32(prgrm))
	outputs[0].Set_ui64(prgrm, outV0)
}

// The built-in f32 function returns operand 1 casted from type i32 to type f32.
func OpI32ToF32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := float32(inputs[0].Get_i32(prgrm))
	outputs[0].Set_f32(prgrm, outV0)
}

// The built-in f64 function returns operand 1 casted from type i32 to type f64.
func OpI32ToF64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := float64(inputs[0].Get_i32(prgrm))
	outputs[0].Set_f64(prgrm, outV0)
}

// The print built-in function formats its arguments and prints them.
func OpI32Print(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	printFormattedLine(prgrm, strconv.AppendInt(formatBuffer(prgrm), int64(inputs[0].Get_i32(prgrm)), 10))
}

// The built-in add function returns the sum of two i32 numbers.
func OpI32Add(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) + inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in sub function returns the difference of two i32 numbers.
func OpI32Sub(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) - inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in neg function returns the opposite of operand 1.
func OpI32Neg(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := -inputs[0].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in mul function returns the product of two i32 numbers.
func OpI32Mul(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) * inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in div function returns the quotient of two i32 numbers.
func OpI32Div(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) / inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in abs function returns the absolute number of the number.
func OpI32Abs(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_i32(prgrm)
	sign := inpV0 >> 31
	outV0 := (inpV0 ^ sign) - sign
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in gt function returns true if operand 1 is greater than operand 2.
func OpI32Gt(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) > inputs[1].Get_i32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in gteq function returns true if operand 1 is greater than or
// equal to operand 2.
func OpI32Gteq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) >= inputs[1].Get_i32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in lt function returns true if operand 1 is less than operand 2.
func OpI32Lt(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) < inputs[1].Get_i32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in lteq function returns true if operand 1 is less than or equal
// to operand 1.
func OpI32Lteq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) <= inputs[1].Get_i32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in eq function returns true if operand 1 is equal to operand 2.
func OpI32Eq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) == inputs[1].Get_i32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in uneq function returns true if operand 1 is different from operand 2.
func OpI32Uneq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) != inputs[1].Get_i32(prgrm)
	outputs[0].Set_bool(prgrm, outV0)
}

// The built-in mod function returns the remainder of operand 1 divided by operand 2.
func OpI32Mod(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) % inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in rand function returns a pseudo random number in [operand 1, operand 2).
func OpI32Rand(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	minimum := inputs[0].Get_i32(prgrm)
	maximum := inputs[1].Get_i32(prgrm)

	r := int(maximum - minimum)
	outV0 := int32(0)
	if r > 0 {
		outV0 = int32(rand.Intn(r) + int(minimum))
	}

	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in bitand function returns the bitwise AND of 2 operands.
func OpI32Bitand(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) & inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in bitor function returns the bitwise OR of 2 operands.
func OpI32Bitor(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) | inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in bitxor function returns the bitwise XOR of 2 operands.
func OpI32Bitxor(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) ^ inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in bitclear function returns the bitwise AND NOT of 2 operands.
func OpI32Bitclear(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) &^ inputs[1].Get_i32(prgrm)
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in bitshl function returns bits of operand 1 shifted to the left
// by number of positions specified in operand 2.
func OpI32Bitshl(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) << uint32(inputs[1].Get_i32(prgrm))
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in bitshr function returns bits of operand 1 shifted to the right
// by number of positions specified in operand 2.
func OpI32Bitshr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0 := inputs[0].Get_i32(prgrm) >> uint32(inputs[1].Get_i32(prgrm))
	outputs[0].Set_i32(prgrm, outV0)
}

// The built-in max function returns the biggest of the two operands.
func OpI32Max(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_i32(prgrm)
	inpV1 := inputs[1].Get_i32(prgrm)
	if inpV1 > inpV0 {
		inpV0 = inpV1
	}
	outputs[0].Set_i32(prgrm, inpV0)
}

// The built-in min function returns the smallest of the two operands.
func OpI32Min(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	inpV0 := inputs[0].Get_i32(prgrm)
	inpV1 := inputs[1].Get_i32(prgrm)
	if inpV1 < inpV0 {
		inpV0 = inpV1
	}
	outputs[0].Set_i32(prgrm, inpV0)
}

func OpI32JmpEq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1294.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	res := inputs[0].Get_i32(prgrm) == inputs[1].Get_i32(prgrm)
	if res {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}

func OpI32JmpUnEq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1295.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	res := inputs[0].Get_i32(prgrm) != inputs[1].Get_i32(prgrm)
	if res {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}

func OpI32JmpGt(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1296.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	res := inputs[0].Get_i32(prgrm) > inputs[1].Get_i32(prgrm)
	if res {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}

func OpI32JmpGtEq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1297.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	res := inputs[0].Get_i32(prgrm) >= inputs[1].Get_i32(prgrm)
	if res {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}

func OpI32JmpLt(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1298.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	res := inputs[0].Get_i32(prgrm) < inputs[1].Get_i32(prgrm)
	if res {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}

func OpI32JmpLtEq(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1299.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	res := inputs[0].Get_i32(prgrm) <= inputs[1].Get_i32(prgrm)
	if res {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}

func OpI32JmpZero(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1300.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	res := inputs[0].Get_i32(prgrm) == 0
	if res {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}

func OpI32JmpNotZero(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	call := prgrm.GetCurrentCall()
	expr := inputs[0].Expr

	// OBJMODEL: state.CXAtomicOperatorMng.CXAtomicOperatorGet(gid), phase 5 #342.
	// Now an arena getter, site S1301.
	cxAtomicOp, err := prgrm.GetCXAtomicOp(expr.Index)
	if err != nil {
		panic(err)
	}

	res := inputs[0].Get_i32(prgrm) != 0
	if res {
		call.Line = call.Line + cxAtomicOp.ThenLines
	} else {
		call.Line = call.Line + cxAtomicOp.ElseLines
	}
}
