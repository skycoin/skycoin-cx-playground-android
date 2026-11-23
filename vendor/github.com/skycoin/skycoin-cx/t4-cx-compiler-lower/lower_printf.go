package lower

import (
	"fmt"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t2-cx-runtime-ir"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// printfStep is one piece of a printf: constant text, or the value of input
// printfStepInput printed by printfStepInstr, which reads
// printfStepSize bytes.
type printfStep struct {
	printfStepText  []byte
	printfStepInput int
	printfStepInstr ir.Instr
	printfStepSize  uint32
}

// printf lowers a printf whose format is a str literal. The format is read
// at lower time as buildString in t4-opcodes reads it, so the image holds
// constant text and one print per value.
func (s *state) printf(code []ir.Instr, inputs []ast.CXTypeSignatureIndex, outputs []ast.CXTypeSignatureIndex) ([]ir.Instr, ReasonCode) {
	if len(inputs) == 0 || len(outputs) != 0 {
		return code, ReasonOperandCount
	}
	format, reason := s.printfFormat(inputs[0])
	if reason != ReasonNone {
		return code, reason
	}
	steps, reason := s.printfSteps(format, inputs)
	if reason != ReasonNone {
		return code, reason
	}

	// The old loop computes the address of every input before printing.
	values := make([]operand, len(inputs))
	for i := 1; i < len(inputs); i++ {
		var p place
		if code, p, reason = s.placeOf(code, inputs[i]); reason != ReasonNone {
			return code, reason
		}
		size := uint32(0)
		for _, step := range steps {
			if step.printfStepText == nil && step.printfStepInput == i {
				size = step.printfStepSize
			}
		}
		if size == 0 {
			continue
		}
		p.placeSize = size
		if code, values[i], reason = s.placeRead(code, p); reason != ReasonNone {
			return code, reason
		}
	}

	for _, step := range steps {
		if step.printfStepText != nil {
			start := uint32(len(s.stateImage.ImageText))
			s.stateImage.ImageText = append(s.stateImage.ImageText, step.printfStepText...)
			code = append(code, ir.Instr{InstrOpcode: ir.OpcodePrintText, InstrN: start, InstrM: uint32(len(step.printfStepText))})
			continue
		}
		in := step.printfStepInstr
		operandSet(&in, 0, values[step.printfStepInput])
		code = append(code, in)
	}
	return code, ReasonNone
}

// printfFormat returns the bytes of a str literal in the data segment.
func (s *state) printfFormat(signatureIndex ast.CXTypeSignatureIndex) ([]byte, ReasonCode) {
	prgrm := s.statePrgrm
	signature := prgrm.GetCXTypeSignatureFromArray(signatureIndex)
	name := signature.Name
	if signature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
		name = prgrm.GetCXArgFromArray(ast.CXArgumentIndex(signature.Meta)).Name
	}
	code, p, reason := s.placeOf(nil, signatureIndex)
	if reason != ReasonNone || name != stringpool.StringGidNil || len(code) > 0 || p.placeFrame {
		return nil, ReasonPrintfFormat
	}
	if _, reason := s.placeStatic(place{placeOffset: p.placeOffset, placeSize: machinePointerSize}); reason != ReasonNone {
		return nil, ReasonPrintfFormat
	}
	object := uint64(types.Read_ptr(prgrm.Memory, types.Pointer(p.placeOffset)))
	if object < uint64(s.stateDataStart) || object+uint64(types.OBJECT_HEADER_SIZE) > uint64(s.stateDataEnd) {
		return nil, ReasonPrintfFormat
	}
	objectSize := uint64(types.Read_obj_size(prgrm.Memory, types.Pointer(object)))
	if objectSize < uint64(types.OBJECT_HEADER_SIZE) || object+objectSize > uint64(s.stateDataEnd) {
		return nil, ReasonPrintfFormat
	}
	return prgrm.Memory[object+uint64(types.OBJECT_HEADER_SIZE) : object+objectSize], ReasonNone
}

// printfSteps walks format as buildString does and returns the pieces to
// print. Text pieces next to each other are joined.
func (s *state) printfSteps(format []byte, inputs []ast.CXTypeSignatureIndex) ([]printfStep, ReasonCode) {
	var steps []printfStep
	text := func(piece []byte) {
		if n := len(steps); n > 0 && steps[n-1].printfStepText != nil {
			steps[n-1].printfStepText = append(steps[n-1].printfStepText, piece...)
			return
		}
		steps = append(steps, printfStep{printfStepText: append([]byte{}, piece...)})
	}

	specifiers := 0
	for c := 0; c < len(format); c++ {
		var next byte
		ch := format[c]
		if c < len(format)-1 {
			next = format[c+1]
		}
		if ch == '\\' {
			switch next {
			case '%':
				c++
				text([]byte{'%'})
			case 'n':
				c++
				text([]byte{'\n'})
			default:
				text([]byte{ch})
			}
			continue
		}
		if ch != '%' {
			text([]byte{ch})
			continue
		}
		if specifiers+1 == len(inputs) {
			text([]byte(fmt.Sprintf("%%!%c(MISSING)", next)))
			c++
			continue
		}
		step, printed, reason := s.printfVerb(next, inputs[specifiers+1])
		if reason != ReasonNone {
			return nil, reason
		}
		if printed {
			step.printfStepInput = specifiers + 1
			steps = append(steps, step)
		}
		c++
		specifiers++
	}

	if specifiers != len(inputs)-1 {
		text([]byte("%!(EXTRA "))
		extra := inputs[specifiers+1:]
		for c, signatureIndex := range extra {
			signature := s.statePrgrm.GetCXTypeSignatureFromArray(signatureIndex)
			text([]byte(signature.GetValueTypeName(s.statePrgrm) + "="))
			step, _, reason := s.printfVerb('v', signatureIndex)
			if reason != ReasonNone {
				return nil, reason
			}
			step.printfStepInput = specifiers + 1 + c
			steps = append(steps, step)
			if c == len(extra)-1 {
				text([]byte(")"))
			} else {
				text([]byte(", "))
			}
		}
	}
	return steps, ReasonNone
}

// printfVerb returns the print of one value for verb, false when buildString
// prints nothing for that verb and type.
func (s *state) printfVerb(verb byte, signatureIndex ast.CXTypeSignatureIndex) (printfStep, bool, ReasonCode) {
	typ := s.printfType(signatureIndex)
	value := func(typ types.Code, verb byte) (printfStep, bool, ReasonCode) {
		return printfStep{
			printfStepInstr: ir.Instr{InstrOpcode: ir.OpcodePrintValue, InstrM: uint32(typ), InstrN: uint32(verb)},
			printfStepSize:  pointerToUint32(typ.Size()),
		}, true, ReasonNone
	}
	str := func(mode ir.PrintStrMode) (printfStep, bool, ReasonCode) {
		return printfStep{
			printfStepInstr: ir.Instr{InstrOpcode: ir.OpcodePrintStr, InstrN: uint32(mode)},
			printfStepSize:  machinePointerSize,
		}, true, ReasonNone
	}
	switch verb {
	case 's':
		if typ != types.STR {
			return printfStep{}, false, ReasonPrintfFormat
		}
		return str(ir.PrintStrModeEscaped)
	case 'd':
		switch typ {
		case types.I8, types.I16, types.I32, types.I64, types.UI8, types.UI16, types.UI32, types.UI64:
			return value(typ, 'd')
		}
		return printfStep{}, false, ReasonNone
	case 'f':
		switch typ {
		case types.F32, types.F64:
			return value(typ, 'f')
		}
		return printfStep{}, false, ReasonNone
	case 'b':
		return value(types.BOOL, 'b')
	case 'v':
		printable, reason := s.printfPrintableType(signatureIndex)
		if reason != ReasonNone {
			return printfStep{}, false, reason
		}
		if printable == types.STR {
			return str(ir.PrintStrModeRaw)
		}
		return value(printable, 'v')
	}
	return printfStep{}, false, ReasonNone
}

// printfType is the CXValue.Type processBuiltInOperators gives an input.
func (s *state) printfType(signatureIndex ast.CXTypeSignatureIndex) types.Code {
	signature := s.statePrgrm.GetCXTypeSignatureFromArray(signatureIndex)
	if signature.Type != ast.TYPE_CXARGUMENT_DEPRECATE {
		return signature.GetType(s.statePrgrm)
	}
	arg := s.statePrgrm.GetCXArgFromArray(ast.CXArgumentIndex(signature.Meta))
	if arg.Type == types.POINTER {
		return arg.PointerTargetType
	}
	return arg.Type
}

// printfPrintableType returns the type GetPrintableValue prints with
// readAtomic, the only case of %v the machine prints. Whole arrays and
// structs, pointers and addresses stay outside.
func (s *state) printfPrintableType(signatureIndex ast.CXTypeSignatureIndex) (types.Code, ReasonCode) {
	prgrm := s.statePrgrm
	signature := prgrm.GetCXTypeSignatureFromArray(signatureIndex)
	var typ types.Code
	switch signature.Type {
	case ast.TYPE_ATOMIC:
		if signature.PassBy == constants.PASSBY_REFERENCE {
			return 0, ReasonPrintfFormat
		}
		typ = types.Code(signature.Meta)
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		element := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(signature.Meta)).GetAssignmentElement(prgrm)
		var reason ReasonCode
		if typ, reason = printfElementType(element); reason != ReasonNone {
			return 0, reason
		}
	case ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC:
		details := prgrm.GetCXTypeSignatureArrayFromArray(signature.Meta)
		if signature.Type == ast.TYPE_POINTER_ARRAY_ATOMIC && !signature.IsDeref || len(details.Indexes) != len(details.Lengths) {
			return 0, ReasonPrintfFormat
		}
		typ = types.Code(details.Meta)
	case ast.TYPE_STRUCT, ast.TYPE_POINTER_STRUCT:
		fields := prgrm.GetCXTypeSignatureStructFromArray(signature.Meta).Fields
		if signature.Type == ast.TYPE_POINTER_STRUCT && !signature.IsDeref || len(fields) == 0 {
			return 0, ReasonPrintfFormat
		}
		var reason ReasonCode
		if typ, reason = printfElementType(prgrm.GetCXArgFromArray(fields[len(fields)-1])); reason != ReasonNone {
			return 0, reason
		}
	default:
		return 0, ReasonPrintfFormat
	}
	switch typ {
	case types.BOOL, types.STR, types.I8, types.I16, types.I32, types.I64,
		types.UI8, types.UI16, types.UI32, types.UI64, types.F32, types.F64:
		return typ, ReasonNone
	}
	return 0, ReasonPrintfFormat
}

// printfElementType returns the type argValueAt prints element with when
// that is a single value.
func printfElementType(element *ast.CXArgument) (types.Code, ReasonCode) {
	typ := element.Type
	if element.IsPointer() {
		for _, operation := range element.DereferenceOperations {
			if operation == constants.DEREF_POINTER {
				typ = element.PointerTargetType
				break
			}
		}
	}
	dimensions := element.Lengths
	if len(element.Indexes) <= len(dimensions) {
		dimensions = dimensions[len(element.Indexes):]
	}
	if len(dimensions) > 0 || element.StructType != nil {
		return 0, ReasonPrintfFormat
	}
	return typ, ReasonNone
}
