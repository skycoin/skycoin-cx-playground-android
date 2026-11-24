package opcodes

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates strs, which
	// adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// VIOLATION STRHOT-099: O, the opcode works on Go string copies of CX strs, #412.
func opStrToI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseInt(inputs[0].Get_str(prgrm), 10, 8)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i8(prgrm, int8(outV0))
}

// VIOLATION STRHOT-100: O, the opcode works on Go string copies of CX strs, #412.
func opStrToI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseInt(inputs[0].Get_str(prgrm), 10, 16)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i16(prgrm, int16(outV0))
}

// VIOLATION STRHOT-101: O, the opcode works on Go string copies of CX strs, #412.
func opStrToI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseInt(inputs[0].Get_str(prgrm), 10, 32)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i32(prgrm, int32(outV0))
}

// VIOLATION STRHOT-102: O, the opcode works on Go string copies of CX strs, #412.
func opStrToI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseInt(inputs[0].Get_str(prgrm), 10, 64)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i64(prgrm, int64(outV0))
}

// VIOLATION STRHOT-103: O, the opcode works on Go string copies of CX strs, #412.
func opStrToUI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseUint(inputs[0].Get_str(prgrm), 10, 8)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui8(prgrm, uint8(outV0))
}

// VIOLATION STRHOT-104: O, the opcode works on Go string copies of CX strs, #412.
func opStrToUI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseUint(inputs[0].Get_str(prgrm), 10, 16)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui16(prgrm, uint16(outV0))
}

// VIOLATION STRHOT-105: O, the opcode works on Go string copies of CX strs, #412.
func opStrToUI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseUint(inputs[0].Get_str(prgrm), 10, 32)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui32(prgrm, uint32(outV0))
}

// VIOLATION STRHOT-106: O, the opcode works on Go string copies of CX strs, #412.
func opStrToUI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseUint(inputs[0].Get_str(prgrm), 10, 64)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui64(prgrm, uint64(outV0))
}

// VIOLATION STRHOT-107: O, the opcode works on Go string copies of CX strs, #412.
func opStrToF32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseFloat(inputs[0].Get_str(prgrm), 32)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_f32(prgrm, float32(outV0))
}

// VIOLATION STRHOT-108: O, the opcode works on Go string copies of CX strs, #412.
func opStrToF64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseFloat(inputs[0].Get_str(prgrm), 64)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_f64(prgrm, float64(outV0))
}

// VIOLATION STRHOT-109: O, the opcode works on Go string copies of CX strs, #412.
func opStrPrint(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	fmt.Println(inputs[0].Get_str(prgrm))
}

// VIOLATION STRHOT-110: O, the opcode works on Go string copies of CX strs, #412.
func opStrConcat(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str(prgrm, inputs[0].Get_str(prgrm)+inputs[1].Get_str(prgrm))
}

// VIOLATION STRHOT-111: O, the opcode works on Go string copies of CX strs, #412.
func opStrSubstr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	str := inputs[0].Get_str(prgrm)
	begin := inputs[1].Get_i32(prgrm)
	end := inputs[2].Get_i32(prgrm)
	if begin < 0 || end < begin || int(end) > len(str) {
		panic(types.CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE)
	}
	outputs[0].Set_str(prgrm, str[begin:end])
}

// VIOLATION STRHOT-112: O, the opcode works on Go string copies of CX strs, #412.
func opStrIndex(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	str := inputs[0].Get_str(prgrm)
	substr := inputs[1].Get_str(prgrm)
	outputs[0].Set_i32(prgrm, int32(strings.Index(str, substr)))
}

// VIOLATION STRHOT-113: O, the opcode works on Go string copies of CX strs, #412.
func opStrLastIndex(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	str := inputs[0].Get_str(prgrm)
	substr := inputs[1].Get_str(prgrm)
	outputs[0].Set_i32(prgrm, int32(strings.LastIndex(str, substr)))
}

// VIOLATION STRHOT-114: O, the opcode works on Go string copies of CX strs, #412.
func opStrTrimSpace(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str(prgrm, strings.TrimSpace(inputs[0].Get_str(prgrm)))
}

// VIOLATION STRHOT-115: O, the opcode works on Go string copies of CX strs, #412.
func opStrEqualByValue(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, inputs[0].Get_str(prgrm) == inputs[1].Get_str(prgrm))
}

func opStrEqualByPointer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, inputs[0].Get_ptr(prgrm) == inputs[1].Get_ptr(prgrm))
}

// VIOLATION STRHOT-116: O, the opcode works on Go string copies of CX strs, #412.
func opStrCompareByValue(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(strings.Compare(inputs[0].Get_str(prgrm), inputs[1].Get_str(prgrm))))
}

// VIOLATION STRHOT-117: O, the opcode works on Go string copies of CX strs, #412.
func opStrByteLength(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(len(inputs[0].Get_str(prgrm))))
}

// opStrBytesOfString returns a new []ui8 holding the bytes of a str, as
// []byte(s) in Go.
// GCREACH: allocates the output byte slice, which adds to the GC pressure,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-318: O, the opcode works on Go string copies of CX strs, #412.
func opStrBytesOfString(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	str := inputs[0].Get_str(prgrm)
	count := types.Cast_int_to_ptr(len(str))
	if count == 0 {
		outputs[0].Set_ptr(prgrm, 0)
		return
	}
	if count > ast.SliceElementLimit(1) {
		panic(constants.HEAP_EXHAUSTED_ERROR)
	}
	slice := ast.SliceResizeEx(prgrm, 0, count, 1)
	copy(ast.GetSliceData(prgrm, slice, 1), str)
	outputs[0].Set_ptr(prgrm, slice)
}

// opStrStringOfBytes returns a new str holding the bytes of a []ui8, as
// string(b) in Go.
// VIOLATION STRHOT-319: O, the opcode works on Go string copies of CX strs, #412.
func opStrStringOfBytes(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str(prgrm, string(inputs[0].GetSlice_ui8(prgrm)))
}
