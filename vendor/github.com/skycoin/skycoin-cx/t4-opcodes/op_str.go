package opcodes

import (
	"bytes"
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t0-util"
	"github.com/skycoin/skycoin-cx/t1-constants"
	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates strs, which
	// adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// The str opcodes read strs as views into program memory and write new strs
// straight into it, with no Go string in between (#412).

func opStrToI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalInt(inputs[0].Get_str_bytes(prgrm), 8)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i8(prgrm, int8(outV0))
}

func opStrToI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalInt(inputs[0].Get_str_bytes(prgrm), 16)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i16(prgrm, int16(outV0))
}

func opStrToI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalInt(inputs[0].Get_str_bytes(prgrm), 32)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i32(prgrm, int32(outV0))
}

func opStrToI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalInt(inputs[0].Get_str_bytes(prgrm), 64)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i64(prgrm, outV0)
}

func opStrToUI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalUint(inputs[0].Get_str_bytes(prgrm), 8)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui8(prgrm, uint8(outV0))
}

func opStrToUI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalUint(inputs[0].Get_str_bytes(prgrm), 16)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui16(prgrm, uint16(outV0))
}

func opStrToUI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalUint(inputs[0].Get_str_bytes(prgrm), 32)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui32(prgrm, uint32(outV0))
}

func opStrToUI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalUint(inputs[0].Get_str_bytes(prgrm), 64)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui64(prgrm, outV0)
}

// VIOLATION STRHOT-107: O, ParseFloat reads a string view of the str, with no copy, #412.
func opStrToF32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseFloat(util.ProgramTextStringView(inputs[0].Get_str_bytes(prgrm)), 32)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_f32(prgrm, float32(outV0))
}

// VIOLATION STRHOT-108: O, ParseFloat reads a string view of the str, with no copy, #412.
func opStrToF64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, err := strconv.ParseFloat(util.ProgramTextStringView(inputs[0].Get_str_bytes(prgrm)), 64)
	if err != nil {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_f64(prgrm, float64(outV0))
}

// VIOLATION VMALLOC-022: appending grows the format buffer past its size, once, #407.
func opStrPrint(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	printFormattedLine(prgrm, append(formatBuffer(prgrm), inputs[0].Get_str_bytes(prgrm)...))
}

func opStrConcat(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str_concat(prgrm, inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm))
}

func opStrSubstr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	str := inputs[0].Get_str_bytes(prgrm)
	begin := inputs[1].Get_i32(prgrm)
	end := inputs[2].Get_i32(prgrm)
	if begin < 0 || end < begin || int(end) > len(str) {
		panic(types.CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE)
	}
	outputs[0].Set_str_bytes(prgrm, str[begin:end])
}

func opStrIndex(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(bytes.Index(inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm))))
}

func opStrLastIndex(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(bytes.LastIndex(inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm))))
}

func opStrTrimSpace(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str_bytes(prgrm, bytes.TrimSpace(inputs[0].Get_str_bytes(prgrm)))
}

func opStrEqualByValue(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, bytes.Equal(inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm)))
}

func opStrEqualByPointer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, inputs[0].Get_ptr(prgrm) == inputs[1].Get_ptr(prgrm))
}

func opStrCompareByValue(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(bytes.Compare(inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm))))
}

func opStrByteLength(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(len(inputs[0].Get_str_bytes(prgrm))))
}

// opStrBytesOfString returns a new []ui8 holding the bytes of a str, as
// []byte(s) in Go.
// GCREACH: allocates the output byte slice, which adds to the GC pressure,
// see docs/GarbageCollector.md.
func opStrBytesOfString(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	str := inputs[0].Get_str_bytes(prgrm)
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
func opStrStringOfBytes(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str_bytes(prgrm, ast.GetSliceData(prgrm, inputs[0].Get_ptr(prgrm), 1))
}
