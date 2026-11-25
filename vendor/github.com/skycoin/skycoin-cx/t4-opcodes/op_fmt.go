package opcodes

import (
	"os"
	"strconv"
	"unicode/utf8"

	"github.com/skycoin/skycoin-cx/t0-types"
	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates strs, which
	// adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// buildString appends the text printf and sprintf make of inputs to res and
// returns it. %v appends the value with ast.AppendPrintableValue.
// VIOLATION VMALLOC-022: appending grows the format buffer past its size, once, #407.
func buildString(prgrm *ast.CXProgram, inputs []ast.CXValue, res []byte) []byte {
	fmtStr := inputs[0].Get_str_bytes(prgrm)

	var specifiersCounter int
	var lenStr = int(len(fmtStr))

	for c := 0; c < len(fmtStr); c++ {
		var nextCh byte
		ch := fmtStr[c]
		if c < lenStr-1 {
			nextCh = fmtStr[c+1]
		}
		if ch == '%' && nextCh == '%' {
			c++
			res = append(res, '%')
			continue
		}
		if ch == '%' {
			if specifiersCounter+1 == len(inputs) {
				res = append(utf8.AppendRune(append(res, "%!"...), rune(nextCh)), "(MISSING)"...)
				c++
				continue
			}

			inp := &inputs[specifiersCounter+1]
			if nextCh == '+' && c+2 < lenStr && fmtStr[c+2] == 'v' {
				// %+v prints as %v, whose structs name their fields.
				c++
				nextCh = 'v'
			}
			switch nextCh {
			case 's':
				res = append(res, inp.Get_str_bytes(prgrm)...)
			case 'd':
				switch inp.Type {
				case types.I8:
					res = strconv.AppendInt(res, int64(inp.Get_i8(prgrm)), 10)
				case types.I16:
					res = strconv.AppendInt(res, int64(inp.Get_i16(prgrm)), 10)
				case types.I32:
					res = strconv.AppendInt(res, int64(inp.Get_i32(prgrm)), 10)
				case types.I64:
					res = strconv.AppendInt(res, inp.Get_i64(prgrm), 10)
				case types.UI8:
					res = strconv.AppendUint(res, uint64(inp.Get_ui8(prgrm)), 10)
				case types.UI16:
					res = strconv.AppendUint(res, uint64(inp.Get_ui16(prgrm)), 10)
				case types.UI32:
					res = strconv.AppendUint(res, uint64(inp.Get_ui32(prgrm)), 10)
				case types.UI64:
					res = strconv.AppendUint(res, inp.Get_ui64(prgrm), 10)
				}
			case 'f':
				switch inp.Type {
				case types.F32:
					res = strconv.AppendFloat(res, float64(inp.Get_f32(prgrm)), 'f', 7, 32)
				case types.F64:
					res = strconv.AppendFloat(res, inp.Get_f64(prgrm), 'f', 16, 64)
				}
			case 'v':
				res = ast.AppendPrintableValue(res, prgrm, inp.FramePointer, inp.TypeSignature)
			case 'b':
				res = strconv.AppendBool(res, inp.Get_bool(prgrm))
			}
			c++
			specifiersCounter++
		} else {
			res = append(res, ch)
		}
	}

	if specifiersCounter != len(inputs)-1 {
		res = append(res, "%!(EXTRA "...)
		lInps := len(inputs[specifiersCounter+1:])
		for c := 0; c < lInps; c++ {
			inp := &inputs[specifiersCounter+1+c]
			res = inp.TypeSignature.AppendValueTypeName(res, prgrm)
			res = append(res, '=')
			res = ast.AppendPrintableValue(res, prgrm, inp.FramePointer, inp.TypeSignature)
			if c != lInps-1 {
				res = append(res, ", "...)
			}
		}
		res = append(res, ')')
	}

	return res
}

func OpSprintf(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	formatted := buildString(prgrm, inputs, formatBuffer(prgrm))
	outputs[0].Set_str_bytes(prgrm, formatted)
	formatBufferKeep(prgrm, formatted)
}

func OpPrintf(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	formatted := buildString(prgrm, inputs, formatBuffer(prgrm))
	os.Stdout.Write(formatted)
	formatBufferKeep(prgrm, formatted)
}
