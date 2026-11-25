package opcodes

import (
	"bytes"

	"github.com/skycoin/skycoin-cx/t0-cx-core-floatparse"
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates strs, which
	// adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// The str opcodes read strs as views into program memory and write new strs
// straight into it, with no Go string in between (#412).

func OpStrToI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalInt(inputs[0].Get_str_bytes(prgrm), 8)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i8(prgrm, int8(outV0))
}

func OpStrToI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalInt(inputs[0].Get_str_bytes(prgrm), 16)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i16(prgrm, int16(outV0))
}

func OpStrToI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalInt(inputs[0].Get_str_bytes(prgrm), 32)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i32(prgrm, int32(outV0))
}

func OpStrToI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalInt(inputs[0].Get_str_bytes(prgrm), 64)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_i64(prgrm, outV0)
}

func OpStrToUI8(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalUint(inputs[0].Get_str_bytes(prgrm), 8)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui8(prgrm, uint8(outV0))
}

func OpStrToUI16(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalUint(inputs[0].Get_str_bytes(prgrm), 16)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui16(prgrm, uint16(outV0))
}

func OpStrToUI32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalUint(inputs[0].Get_str_bytes(prgrm), 32)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui32(prgrm, uint32(outV0))
}

func OpStrToUI64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, ok := parseDecimalUint(inputs[0].Get_str_bytes(prgrm), 64)
	if !ok {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_ui64(prgrm, outV0)
}

func OpStrToF32(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, result := floatparse.FloatParseBytes(inputs[0].Get_str_bytes(prgrm), 32)
	if result != floatparse.FloatParseResultOK {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_f32(prgrm, float32(outV0))
}

func OpStrToF64(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outV0, result := floatparse.FloatParseBytes(inputs[0].Get_str_bytes(prgrm), 64)
	if result != floatparse.FloatParseResultOK {
		panic(types.CX_RUNTIME_ERROR)
	}
	outputs[0].Set_f64(prgrm, float64(outV0))
}

// VIOLATION VMALLOC-022: appending grows the format buffer past its size, once, #407.
func OpStrPrint(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	printFormattedLine(prgrm, append(formatBuffer(prgrm), inputs[0].Get_str_bytes(prgrm)...))
}

func OpStrConcat(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str_concat(prgrm, inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm))
}

func OpStrSubstr(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	str := inputs[0].Get_str_bytes(prgrm)
	begin := inputs[1].Get_i32(prgrm)
	end := inputs[2].Get_i32(prgrm)
	if begin < 0 || end < begin || int(end) > len(str) {
		panic(types.CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE)
	}
	outputs[0].Set_str_bytes(prgrm, str[begin:end])
}

func OpStrIndex(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(bytes.Index(inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm))))
}

func OpStrLastIndex(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(bytes.LastIndex(inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm))))
}

func OpStrTrimSpace(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str_bytes(prgrm, bytes.TrimSpace(inputs[0].Get_str_bytes(prgrm)))
}

func OpStrEqualByValue(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, bytes.Equal(inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm)))
}

func OpStrEqualByPointer(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_bool(prgrm, inputs[0].Get_ptr(prgrm) == inputs[1].Get_ptr(prgrm))
}

func OpStrCompareByValue(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(bytes.Compare(inputs[0].Get_str_bytes(prgrm), inputs[1].Get_str_bytes(prgrm))))
}

func OpStrByteLength(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i32(prgrm, int32(len(inputs[0].Get_str_bytes(prgrm))))
}

// OpStrBytesOfString returns a new []ui8 holding the bytes of a str, as
// []byte(s) in Go.
// GCREACH: allocates the output byte slice, which adds to the GC pressure,
// see docs/GarbageCollector.md.
func OpStrBytesOfString(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	str := inputs[0].Get_str_bytes(prgrm)
	count := types.Cast_int_to_ptr(len(str))
	if count == 0 {
		outputs[0].Set_ptr(prgrm, 0)
		return
	}
	if count > ast.SliceElementLimit(prgrm, 1) {
		ast.HeapOutOfMemory(prgrm, uint64(types.OBJECT_HEADER_SIZE+constants.SLICE_HEADER_SIZE)+uint64(count))
	}
	slice := ast.SliceResizeEx(prgrm, 0, count, 1)
	copy(ast.GetSliceData(prgrm, slice, 1), str)
	outputs[0].Set_ptr(prgrm, slice)
}

// OpStrStringOfBytes returns a new str holding the bytes of a []ui8, as
// string(b) in Go.
func OpStrStringOfBytes(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_str_bytes(prgrm, ast.GetSliceData(prgrm, inputs[0].Get_ptr(prgrm), 1))
}
