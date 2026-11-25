// Package lower turns a compiled CXProgram into an ir.Image, steps 4 and 6
// of skycoin/skycoin-cx#408. It lowers integer, float and bool operators,
// conversions, print, string.PrintLine, printf with a literal format,
// identity, declarations, jumps, test and calls to other functions. An
// operand may index arrays, select struct fields and dereference pointers
// into the stack and data segment, as GetFinalOffset does. Slices and the
// heap stay outside. A function using anything else is reported as a Failure
// with a reason code, and its ir.Func gets no code.
package lower

import (
	"fmt"

	assert "github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t2-cx-runtime-ir"
	// GCREACH: t3-ast owns the heap and the GC roots. Lowering rejects heap operands,
	// so vm code never reaches the GC, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
)

// ReasonCode says why a function did not lower.
type ReasonCode uint32

const (
	ReasonNone ReasonCode = iota
	// ReasonOpcodeOutsideSubset is a built-in outside the subset,
	// FailureOpcode names it.
	ReasonOpcodeOutsideSubset
	// ReasonOperandNotScalar is an operand of a type signature the lowering
	// does not know.
	ReasonOperandNotScalar
	// ReasonOperandIndexed is an index, field or dereference the lowering
	// does not follow, for example more indexes than array dimensions.
	ReasonOperandIndexed
	// ReasonOperandOutsideData is an absolute operand outside the data
	// segment.
	ReasonOperandOutsideData
	// ReasonOperandByReference is a reference to an inner element through a
	// pointer, which the old loop moves back by the object header.
	ReasonOperandByReference
	// ReasonOperandString is a str compared by test.
	ReasonOperandString
	// ReasonOperandCount is an operator with an unexpected operand count.
	ReasonOperandCount
	// ReasonDeclarationType is a declaration of a type the old loop panics on.
	ReasonDeclarationType
	// ReasonExpressionType is an expression that is neither an atomic
	// operator nor a line.
	ReasonExpressionType
	// ReasonCalleeLayout is a call to a function whose inputs or outputs are
	// not plain frame variables.
	ReasonCalleeLayout
	// ReasonParameterLayout is a function whose own inputs or outputs are
	// not plain frame variables.
	ReasonParameterLayout
	// ReasonOperandHeap is an operand whose address goes through a slice or
	// an escaped local, which live on the heap.
	ReasonOperandHeap
	// ReasonPrintfFormat is a printf whose format is not a str literal, or a
	// verb and value the machine does not print as buildString does.
	ReasonPrintfFormat
	// ReasonOperandOutsideFrame is a stack operand that does not fit the frame
	// of its function.
	ReasonOperandOutsideFrame
	// ReasonLibraryFunctionOutsideSubset is a call of a system library function
	// that has no vm form, see vm.LibraryFunctionVmLinked.
	ReasonLibraryFunctionOutsideSubset
	// ReasonCount is one past the last reason.
	ReasonCount
)

// ReasonNames names each ReasonCode for reports. Read only.
var ReasonNames = [ReasonCount]string{
	ReasonNone:                         "none",
	ReasonOpcodeOutsideSubset:          "opcode outside subset",
	ReasonOperandNotScalar:             "operand not scalar",
	ReasonOperandIndexed:               "operand indexed",
	ReasonOperandOutsideData:           "operand outside data",
	ReasonOperandByReference:           "operand by reference",
	ReasonOperandString:                "operand string",
	ReasonOperandCount:                 "operand count",
	ReasonDeclarationType:              "declaration type",
	ReasonExpressionType:               "expression type",
	ReasonCalleeLayout:                 "callee layout",
	ReasonParameterLayout:              "parameter layout",
	ReasonOperandHeap:                  "operand heap",
	ReasonPrintfFormat:                 "printf format",
	ReasonOperandOutsideFrame:          "operand outside frame",
	ReasonLibraryFunctionOutsideSubset: "library function outside subset",
}

// Failure is one function that did not lower. FailureExpression indexes its
// expressions. FailureOpcode is the language opcode for
// ReasonOpcodeOutsideSubset and 0 otherwise.
type Failure struct {
	FailureFunction   uint32
	FailureExpression uint32
	FailureReason     ReasonCode
	FailureOpcode     uint32
}

// operand is one lowered operand, a byte offset the instruction reads or
// writes directly.
type operand struct {
	operandOffset uint32
	operandFrame  bool
	operandSize   uint32
}

// place is where the value of an operand is. A static place is an offset,
// relative to the frame when placeFrame is set. A computed place is the
// address held in the scratch slot placeOffset, plus placeConstant.
type place struct {
	placeOffset   uint32
	placeFrame    bool
	placeComputed bool
	placeConstant uint32
	placeSize     uint32
}

// pendingStore is the copy from scratch to a computed output place, made
// after the instruction that writes the output.
type pendingStore struct {
	pendingStoreActive bool
	pendingStoreInstr  ir.Instr
}

// state is one Program call. stateScratchUsed is the scratch the current
// expression uses, stateFrameSize the frame of the current function.
type state struct {
	statePrgrm       *ast.CXProgram
	stateTable       opcodeTable
	stateStackSize   uint32
	stateDataStart   uint32
	stateDataEnd     uint32
	stateScratchUsed uint32
	stateFrameSize   uint32
	statePrintf      int
	statePrintLine   int
	stateImage       ir.Image
	stateFiles       map[ast.SourceFileGid]uint32
}

// jumpFixup is a jump target still in expression indexes.
type jumpFixup struct {
	jumpFixupPc          uint32
	jumpFixupTrue        int
	jumpFixupFalse       int
	jumpFixupConditional bool
}

// Program lowers every function of prgrm that is not a built-in. Functions
// that do not lower are listed in the failures and have no code. prgrm must
// be compiled and not yet run.
func Program(prgrm *ast.CXProgram) (ir.Image, []Failure) {
	s := &state{
		statePrgrm:     prgrm,
		stateStackSize: pointerToUint32(prgrm.Stack.Size),
		stateDataStart: pointerToUint32(prgrm.Data.StartsAt),
		stateFiles:     map[ast.SourceFileGid]uint32{},
	}
	s.stateDataEnd = s.stateDataStart + pointerToUint32(prgrm.Data.Size)
	opcodeTableBuild(&s.stateTable)
	s.statePrintf = opcodeNumber("printf")
	s.statePrintLine = opcodeNumber("string.PrintLine")

	image := &s.stateImage
	image.ImageStackSize = s.stateStackSize
	image.ImageDataStart = s.stateDataStart
	image.ImageData = append([]byte(nil), prgrm.Memory[s.stateDataStart:s.stateDataEnd]...)
	image.ImageFuncs = make([]ir.Func, len(prgrm.CXFunctions))
	image.ImageFuncNames = make([]string, len(prgrm.CXFunctions))
	image.ImageEntryInit = ir.ImageFuncNone
	image.ImageEntryMain = ir.ImageFuncNone
	image.ImageLibraryEnabled = prgrm.ProgramLibrariesEnabled

	if packageIndex, found := prgrm.Packages[prgrm.CxIrState.CxIrStateNameMain]; found {
		mainPackage, err := prgrm.GetPackageFromArray(packageIndex)
		assert.AssertInvariantTrue(err == nil, "lower: main package index out of range")
		if functionIndex, found := mainPackage.Functions[prgrm.CxIrState.CxIrStateNameSysInit]; found {
			image.ImageEntryInit = uint32(functionIndex)
		}
		if functionIndex, found := mainPackage.Functions[prgrm.CxIrState.CxIrStateNameMain]; found {
			image.ImageEntryMain = uint32(functionIndex)
		}
	}

	var failures []Failure
	for functionIndex := range prgrm.CXFunctions {
		fn := &prgrm.CXFunctions[functionIndex]
		image.ImageFuncNames[functionIndex] = prgrm.CXProgramNameText(fn.Name)
		if fn.IsBuiltIn() {
			continue
		}
		if failure, failed := s.function(uint32(functionIndex), fn); failed {
			failures = append(failures, failure)
		}
	}
	return s.stateImage, failures
}

// opcodeNumber returns the language number of a live opcode name.
func opcodeNumber(name string) int {
	row, found := constants.OpcodeLifecycleLiveByName(name)
	assert.AssertInvariantTrue(found, "lower: opcode "+name+" is not live")
	return row.Number
}

// function lowers fn into stateImage.ImageFuncs[functionIndex].
func (s *state) function(functionIndex uint32, fn *ast.CXFunction) (Failure, bool) {
	failure := Failure{FailureFunction: functionIndex}
	lowered := ir.Func{FuncFrameSize: pointerToUint32(fn.Size)}
	s.stateFrameSize = lowered.FuncFrameSize

	var reason ReasonCode
	if lowered.FuncInputs, reason = s.slots(fn.GetInputs(s.statePrgrm), lowered.FuncFrameSize); reason != ReasonNone {
		failure.FailureReason = ReasonParameterLayout
		return failure, true
	}
	if lowered.FuncOutputs, reason = s.slots(fn.GetOutputs(s.statePrgrm), lowered.FuncFrameSize); reason != ReasonNone {
		failure.FailureReason = ReasonParameterLayout
		return failure, true
	}

	lineCount := fn.LineCount
	if lineCount < 0 || lineCount > len(fn.Expressions) {
		assert.AssertInvariantTrue(false, "lower: LineCount outside the expressions of "+s.statePrgrm.CXProgramNameText(fn.Name))
	}
	expressionPc := make([]uint32, lineCount+1)
	var code []ir.Instr
	var fixups []jumpFixup
	var lines []ir.Line

	for line := 0; line < lineCount; line++ {
		expressionPc[line] = uint32(len(code))
		failure.FailureExpression = uint32(line)
		s.stateScratchUsed = 0
		expr := &fn.Expressions[line]

		switch expr.Type {
		case ast.CX_LINE:
			cxLine, err := s.statePrgrm.GetCXLine(expr.Index)
			assert.AssertInvariantTrue(err == nil, "lower: CX_LINE index out of range")
			lines = append(lines, ir.Line{
				LinePc:     uint32(len(code)),
				LineFile:   s.file(cxLine.FileName),
				LineNumber: uint32(cxLine.LineNumber),
			})
			continue
		case ast.CX_ATOMIC_OPERATOR:
		default:
			failure.FailureReason = ReasonExpressionType
			return failure, true
		}

		cxAtomicOp, err := s.statePrgrm.GetCXAtomicOp(expr.Index)
		assert.AssertInvariantTrue(err == nil, "lower: atomic operator index out of range")
		operator := s.statePrgrm.GetFunctionFromArray(cxAtomicOp.Operator)

		switch {
		case operator == nil:
			code, reason = s.declaration(code, cxAtomicOp)
		case operator.IsBuiltIn():
			var fixup jumpFixup
			var jump bool
			code, fixup, jump, reason, failure.FailureOpcode = s.builtIn(code, line, cxAtomicOp, operator)
			if jump {
				fixups = append(fixups, fixup)
			}
		default:
			code, reason = s.call(code, cxAtomicOp, operator)
		}
		if reason != ReasonNone {
			failure.FailureReason = reason
			return failure, true
		}
	}

	expressionPc[lineCount] = uint32(len(code))
	code = append(code, ir.Instr{InstrOpcode: ir.OpcodeReturn})

	for _, fixup := range fixups {
		in := &code[fixup.jumpFixupPc]
		in.InstrM = expressionPc[jumpTarget(fixup.jumpFixupTrue, lineCount)]
		if fixup.jumpFixupConditional {
			in.InstrN = expressionPc[jumpTarget(fixup.jumpFixupFalse, lineCount)]
		}
	}

	lowered.FuncCode = code
	lowered.FuncLineFirst = uint32(len(s.stateImage.ImageLines))
	lowered.FuncLineCount = uint32(len(lines))
	s.stateImage.ImageLines = append(s.stateImage.ImageLines, lines...)
	s.stateImage.ImageFuncs[functionIndex] = lowered
	return Failure{}, false
}

// jumpTarget clamps a target expression to lineCount, where the old loop
// returns. A negative target would index out of range in the old loop.
func jumpTarget(target int, lineCount int) int {
	assert.AssertInvariantTrue(target >= 0, fmt.Sprintf("lower: jump to expression %d", target))
	if target > lineCount {
		return lineCount
	}
	return target
}

// declaration zeroes a declared variable, as wipeDeclarationMemory does. It
// always adds the frame pointer, also for an offset outside the stack.
func (s *state) declaration(code []ir.Instr, cxAtomicOp *ast.CXAtomicOperator) ([]ir.Instr, ReasonCode) {
	outputs := cxAtomicOp.GetOutputs(s.statePrgrm)
	if len(outputs) == 0 {
		return code, ReasonOperandCount
	}
	signature := s.statePrgrm.GetCXTypeSignatureFromArray(outputs[0])
	var offset types.Pointer
	switch signature.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		offset = s.statePrgrm.GetCXArg(ast.CXArgumentIndex(signature.Meta)).Offset
	case ast.TYPE_ATOMIC, ast.TYPE_POINTER_ATOMIC, ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC,
		ast.TYPE_SLICE_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC, ast.TYPE_STRUCT, ast.TYPE_POINTER_STRUCT:
		offset = signature.Offset
	default:
		return code, ReasonDeclarationType
	}
	if ast.OffsetEscaped(offset) {
		return code, ReasonOperandHeap
	}
	return append(code, ir.Instr{
		InstrOpcode: ir.OpcodeZero,
		InstrMode:   ir.InstrModeFrameA,
		InstrA:      pointerToUint32(offset),
		InstrN:      pointerToUint32(signature.GetSize(s.statePrgrm, false)),
	}), ReasonNone
}

// builtIn lowers a built-in operator. A jump returns its fixup. The old loop
// computes every operand address before it runs the operator, and so does
// the lowered code.
func (s *state) builtIn(code []ir.Instr, line int, cxAtomicOp *ast.CXAtomicOperator, operator *ast.CXFunction) ([]ir.Instr, jumpFixup, bool, ReasonCode, uint32) {
	inputs := cxAtomicOp.GetInputs(s.statePrgrm)
	outputs := cxAtomicOp.GetOutputs(s.statePrgrm)
	opcode := operator.AtomicOPCode
	noFixup := jumpFixup{}

	switch opcode {
	case constants.OP_NOP:
		return code, noFixup, false, ReasonNone, 0
	case constants.OP_GOTO, constants.OP_BREAK, constants.OP_CONTINUE:
		pc := uint32(len(code))
		code = append(code, ir.Instr{InstrOpcode: ir.OpcodeJump})
		return code, jumpFixup{jumpFixupPc: pc, jumpFixupTrue: line + cxAtomicOp.ThenLines + 1}, true, ReasonNone, 0
	case constants.OP_JMP, constants.OP_ABS_JMP:
		if len(inputs) != 1 {
			return code, noFixup, false, ReasonOperandCount, 0
		}
		code, condition, reason := s.read(code, inputs[0])
		if reason != ReasonNone {
			return code, noFixup, false, reason, 0
		}
		pc := uint32(len(code))
		in := ir.Instr{InstrOpcode: ir.OpcodeJumpIf}
		operandSet(&in, 0, condition)
		fixup := jumpFixup{
			jumpFixupPc:          pc,
			jumpFixupTrue:        line + cxAtomicOp.ThenLines + 1,
			jumpFixupFalse:       line + cxAtomicOp.ElseLines + 1,
			jumpFixupConditional: true,
		}
		if opcode == constants.OP_ABS_JMP {
			fixup.jumpFixupTrue = cxAtomicOp.ThenLines + 1
			fixup.jumpFixupFalse = cxAtomicOp.ElseLines + 1
		}
		return append(code, in), fixup, true, ReasonNone, 0
	case constants.OP_IDENTITY:
		code, reason := s.identity(code, inputs, outputs)
		return code, noFixup, false, reason, 0
	case constants.OP_TEST:
		code, reason := s.test(code, inputs)
		return code, noFixup, false, reason, 0
	case s.statePrintLine:
		if len(inputs) != 1 || len(outputs) != 0 {
			return code, noFixup, false, ReasonOperandCount, 0
		}
		code, str, reason := s.read(code, inputs[0])
		if reason != ReasonNone {
			return code, noFixup, false, reason, 0
		}
		in := ir.Instr{InstrOpcode: ir.OpcodePrintStr, InstrN: uint32(ir.PrintStrModeLine)}
		operandSet(&in, 0, str)
		return append(code, in), noFixup, false, ReasonNone, 0
	case s.statePrintf:
		code, reason := s.printf(code, inputs, outputs)
		return code, noFixup, false, reason, 0
	case constants.OP_LIBRARY_CALL:
		code, reason := s.libraryCall(code, cxAtomicOp, operator)
		return code, noFixup, false, reason, 0
	}

	entry := opcodeEntry{}
	if opcode > 0 && opcode < opcodeTableSize {
		entry = s.stateTable[opcode]
	}
	if entry.opcodeEntryOpcode == ir.OpcodeInvalid {
		return code, noFixup, false, ReasonOpcodeOutsideSubset, uint32(opcode)
	}
	if len(inputs) != entry.opcodeEntryInputs || len(outputs) != entry.opcodeEntryOutputs {
		return code, noFixup, false, ReasonOperandCount, 0
	}
	in := ir.Instr{InstrOpcode: entry.opcodeEntryOpcode}
	for i, signatureIndex := range inputs {
		var lowered operand
		var reason ReasonCode
		code, lowered, reason = s.read(code, signatureIndex)
		if reason != ReasonNone {
			return code, noFixup, false, reason, 0
		}
		operandSet(&in, i, lowered)
	}
	var store pendingStore
	if len(outputs) == 1 {
		var lowered operand
		var reason ReasonCode
		code, lowered, store, reason = s.writeBegin(code, outputs[0], 0)
		if reason != ReasonNone {
			return code, noFixup, false, reason, 0
		}
		operandSet(&in, 2, lowered)
	}
	code = append(code, in)
	return writeEnd(code, store), noFixup, false, ReasonNone, 0
}

// identity lowers opIdentity. It either copies the input by value or writes
// its address as a pointer, by the same type tests opIdentity makes.
func (s *state) identity(code []ir.Instr, inputs []ast.CXTypeSignatureIndex, outputs []ast.CXTypeSignatureIndex) ([]ir.Instr, ReasonCode) {
	if len(inputs) != 1 || len(outputs) != 1 {
		return code, ReasonOperandCount
	}
	prgrm := s.statePrgrm
	input := prgrm.GetCXTypeSignatureFromArray(inputs[0])
	output := prgrm.GetCXTypeSignatureFromArray(outputs[0])
	var address bool
	switch output.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		element := prgrm.GetCXArgFromArray(ast.CXArgumentIndex(output.Meta)).GetAssignmentElement(prgrm)
		address = ast.CXArgumentTakesAddress(prgrm, element, input)
		if ast.CXArgumentStringSlicePointer(element) {
			address = ast.CXTypeSignaturePassBy(prgrm, input) == constants.PASSBY_REFERENCE
		}
	case ast.TYPE_ATOMIC, ast.TYPE_SLICE_ATOMIC:
		address = output.PassBy == constants.PASSBY_REFERENCE
	case ast.TYPE_POINTER_ATOMIC:
		address = !ast.CXTypeSignaturePointerValue(prgrm, input)
	case ast.TYPE_ARRAY_ATOMIC:
	case ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_POINTER_SLICE_ATOMIC:
		address = input.PassBy == constants.PASSBY_REFERENCE
	case ast.TYPE_STRUCT, ast.TYPE_POINTER_STRUCT:
		address = ast.CXTypeSignatureStructTakesAddress(prgrm, output, input)
	default:
		return code, ReasonOperandNotScalar
	}

	var source place
	var reason ReasonCode
	if code, source, reason = s.placeOf(code, inputs[0]); reason != ReasonNone {
		return code, reason
	}
	if address {
		code, in, reason := s.addressOf(code, source)
		if reason != ReasonNone {
			return code, reason
		}
		code, destination, store, reason := s.writeBegin(code, outputs[0], machinePointerSize)
		if reason != ReasonNone {
			return code, reason
		}
		operandSet(&in, 2, destination)
		return writeEnd(append(code, in), store), ReasonNone
	}
	code, value, reason := s.placeRead(code, source)
	if reason != ReasonNone {
		return code, reason
	}
	code, destination, store, reason := s.writeBegin(code, outputs[0], value.operandSize)
	if reason != ReasonNone {
		return code, reason
	}
	in := ir.Instr{InstrOpcode: ir.OpcodeMove, InstrN: value.operandSize}
	operandSet(&in, 0, value)
	operandSet(&in, 2, destination)
	return writeEnd(append(code, in), store), ReasonNone
}

// machinePointerSize is the size of a pointer in memory.
const machinePointerSize = uint32(types.POINTER_SIZE)

// addressOf returns an instruction that writes the address of p as a
// pointer, its output operand still to set.
func (s *state) addressOf(code []ir.Instr, p place) ([]ir.Instr, ir.Instr, ReasonCode) {
	if p.placeComputed {
		return code, ir.Instr{InstrOpcode: ir.OpcodeAddressPointer, InstrA: p.placeOffset, InstrM: p.placeConstant}, ReasonNone
	}
	lowered, reason := s.placeStatic(p)
	if reason != ReasonNone {
		return code, ir.Instr{}, reason
	}
	in := ir.Instr{InstrOpcode: ir.OpcodeAddressOf}
	operandSet(&in, 0, lowered)
	return code, in, ReasonNone
}

// test lowers opTest. assert compares a str argument by its characters,
// which stays outside the subset. The str tests are the ones assert makes.
func (s *state) test(code []ir.Instr, inputs []ast.CXTypeSignatureIndex) ([]ir.Instr, ReasonCode) {
	if len(inputs) != 3 {
		return code, ReasonOperandCount
	}
	prgrm := s.statePrgrm
	first := prgrm.GetCXTypeSignatureFromArray(inputs[0])
	switch first.Type {
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		if prgrm.GetCXArgFromArray(ast.CXArgumentIndex(first.Meta)).IsString() {
			return code, ReasonOperandString
		}
	case ast.TYPE_STRUCT, ast.TYPE_POINTER_STRUCT:
		fields := prgrm.GetCXTypeSignatureStructFromArray(first.Meta).Fields
		if len(fields) > 0 && prgrm.GetCXArgFromArray(fields[len(fields)-1]).IsString() {
			return code, ReasonOperandString
		}
	case ast.TYPE_ATOMIC, ast.TYPE_POINTER_ATOMIC, ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC, ast.TYPE_SLICE_ATOMIC:
	default:
		return code, ReasonOperandNotScalar
	}
	in := ir.Instr{InstrOpcode: ir.OpcodeTestEqual}
	for i, signatureIndex := range inputs {
		var lowered operand
		var reason ReasonCode
		code, lowered, reason = s.read(code, signatureIndex)
		if reason != ReasonNone {
			return code, reason
		}
		operandSet(&in, i, lowered)
		switch i {
		case 0:
			in.InstrN = lowered.operandSize
		case 1:
			in.InstrM = lowered.operandSize
		}
	}
	return append(code, in), ReasonNone
}

// call lowers a call of a function, as processNonAtomicOperators and
// popStack do: wipe the callee frame, copy the inputs or their addresses,
// call, then copy the outputs the caller receives. The addresses of the
// outputs are computed after the call, as popStack computes them.
func (s *state) call(code []ir.Instr, cxAtomicOp *ast.CXAtomicOperator, callee *ast.CXFunction) ([]ir.Instr, ReasonCode) {
	prgrm := s.statePrgrm
	inputs := cxAtomicOp.GetInputs(prgrm)
	outputs := cxAtomicOp.GetOutputs(prgrm)
	calleeInputs := callee.GetInputs(prgrm)
	calleeOutputs := callee.GetOutputs(prgrm)
	if len(inputs) > len(calleeInputs) {
		return code, ReasonOperandCount
	}

	calleeFrameSize := pointerToUint32(callee.Size)
	code = append(code, ir.Instr{InstrOpcode: ir.OpcodeCallFrame, InstrN: calleeFrameSize})
	for i, signatureIndex := range inputs {
		destination, reason := s.slot(calleeInputs[i], calleeFrameSize)
		if reason != ReasonNone {
			return code, ReasonCalleeLayout
		}
		signature := prgrm.GetCXTypeSignatureFromArray(signatureIndex)
		byReference := signature.PassBy == constants.PASSBY_REFERENCE
		if signature.Type == ast.TYPE_CXARGUMENT_DEPRECATE {
			byReference = byReference || prgrm.GetCXArgFromArray(ast.CXArgumentIndex(signature.Meta)).PassBy == constants.PASSBY_REFERENCE
		}
		var source place
		if code, source, reason = s.placeOf(code, signatureIndex); reason != ReasonNone {
			return code, reason
		}
		var value operand
		if byReference {
			if signature.IsInnerReference(prgrm) {
				return code, ReasonOperandByReference
			}
			var in ir.Instr
			if code, in, reason = s.addressOf(code, source); reason != ReasonNone {
				return code, reason
			}
			value = operand{operandOffset: s.scratch(machinePointerSize), operandSize: machinePointerSize}
			operandSet(&in, 2, value)
			code = append(code, in)
		} else if code, value, reason = s.placeRead(code, source); reason != ReasonNone {
			return code, reason
		}
		in := ir.Instr{InstrOpcode: ir.OpcodeCallArg, InstrC: destination.SlotOffset, InstrN: value.operandSize}
		operandSet(&in, 0, value)
		code = append(code, in)
	}

	code = append(code, ir.Instr{InstrOpcode: ir.OpcodeCall, InstrM: uint32(cxAtomicOp.Operator)})
	var sources []ir.Slot
	for i, signatureIndex := range calleeOutputs {
		if i >= len(outputs) {
			continue
		}
		source, reason := s.slot(signatureIndex, calleeFrameSize)
		if reason != ReasonNone {
			return code, ReasonCalleeLayout
		}
		sources = append(sources, source)
	}
	return s.callResults(code, outputs, sources)
}

// callResults copies the outputs of the call that just returned, from the
// sources in its frame at sp, to the operands outputs. A computed output
// goes through scratch.
func (s *state) callResults(code []ir.Instr, outputs []ast.CXTypeSignatureIndex, sources []ir.Slot) ([]ir.Instr, ReasonCode) {
	for i, source := range sources {
		var reason ReasonCode
		var tail []ir.Instr
		var destination place
		if tail, destination, reason = s.placeOf(nil, outputs[i]); reason != ReasonNone {
			return code, reason
		}
		in := ir.Instr{InstrOpcode: ir.OpcodeCallResult, InstrA: source.SlotOffset, InstrN: source.SlotSize}
		if !destination.placeComputed {
			lowered, reason := s.placeStatic(destination)
			if reason != ReasonNone {
				return code, reason
			}
			operandSet(&in, 2, lowered)
			code = append(code, in)
			continue
		}
		value := operand{operandOffset: s.scratch(source.SlotSize), operandSize: source.SlotSize}
		operandSet(&in, 2, value)
		code = append(code, in)
		code = append(code, tail...)
		code = append(code, ir.Instr{
			InstrOpcode: ir.OpcodeAddressStore,
			InstrD:      1,
			InstrA:      value.operandOffset,
			InstrC:      destination.placeOffset,
			InstrM:      destination.placeConstant,
			InstrN:      source.SlotSize,
		})
	}
	return code, ReasonNone
}

// slots lowers the inputs or outputs of a function to frame slots.
func (s *state) slots(signatures []ast.CXTypeSignatureIndex, frameSize uint32) ([]ir.Slot, ReasonCode) {
	slots := make([]ir.Slot, len(signatures))
	for i, signatureIndex := range signatures {
		slot, reason := s.slot(signatureIndex, frameSize)
		if reason != ReasonNone {
			return nil, reason
		}
		slots[i] = slot
	}
	return slots, ReasonNone
}

// slot lowers a function input or output, which must be a plain variable
// inside a frame of frameSize bytes.
func (s *state) slot(signatureIndex ast.CXTypeSignatureIndex, frameSize uint32) (ir.Slot, ReasonCode) {
	code, p, reason := s.placeOf(nil, signatureIndex)
	if reason != ReasonNone {
		return ir.Slot{}, reason
	}
	if len(code) > 0 || p.placeComputed || !p.placeFrame || uint64(p.placeOffset)+uint64(p.placeSize) > uint64(frameSize) {
		return ir.Slot{}, ReasonOperandOutsideData
	}
	return ir.Slot{SlotOffset: p.placeOffset, SlotSize: p.placeSize}, ReasonNone
}

// read lowers an input operand. A computed place is loaded into scratch.
func (s *state) read(code []ir.Instr, signatureIndex ast.CXTypeSignatureIndex) ([]ir.Instr, operand, ReasonCode) {
	code, p, reason := s.placeOf(code, signatureIndex)
	if reason != ReasonNone {
		return code, operand{}, reason
	}
	return s.placeRead(code, p)
}

// placeRead returns p as an operand, loading a computed place into scratch.
func (s *state) placeRead(code []ir.Instr, p place) ([]ir.Instr, operand, ReasonCode) {
	if !p.placeComputed {
		lowered, reason := s.placeStatic(p)
		return code, lowered, reason
	}
	lowered := operand{operandOffset: s.scratch(p.placeSize), operandSize: p.placeSize}
	return append(code, ir.Instr{
		InstrOpcode: ir.OpcodeAddressLoad,
		InstrA:      p.placeOffset,
		InstrM:      p.placeConstant,
		InstrN:      p.placeSize,
		InstrC:      lowered.operandOffset,
	}), lowered, ReasonNone
}

// writeBegin lowers an output operand that an instruction writes size bytes
// to, or as many bytes as the operand has when size is 0. A computed place
// is written to scratch and copied to its address by writeEnd.
func (s *state) writeBegin(code []ir.Instr, signatureIndex ast.CXTypeSignatureIndex, size uint32) ([]ir.Instr, operand, pendingStore, ReasonCode) {
	code, p, reason := s.placeOf(code, signatureIndex)
	if reason != ReasonNone {
		return code, operand{}, pendingStore{}, reason
	}
	if size != 0 {
		p.placeSize = size
	}
	if !p.placeComputed {
		lowered, reason := s.placeStatic(p)
		return code, lowered, pendingStore{}, reason
	}
	lowered := operand{operandOffset: s.scratch(p.placeSize), operandSize: p.placeSize}
	return code, lowered, pendingStore{
		pendingStoreActive: true,
		pendingStoreInstr: ir.Instr{
			InstrOpcode: ir.OpcodeAddressStore,
			InstrD:      1,
			InstrA:      lowered.operandOffset,
			InstrC:      p.placeOffset,
			InstrM:      p.placeConstant,
			InstrN:      p.placeSize,
		},
	}, ReasonNone
}

// writeEnd adds the store of writeBegin, if any.
func writeEnd(code []ir.Instr, store pendingStore) []ir.Instr {
	if !store.pendingStoreActive {
		return code
	}
	return append(code, store.pendingStoreInstr)
}

// scratch reserves size bytes of scratch for the current expression and
// returns their absolute address.
func (s *state) scratch(size uint32) uint32 {
	address := s.stateDataEnd + s.stateScratchUsed
	s.stateScratchUsed += size
	if s.stateScratchUsed > s.stateImage.ImageScratchSize {
		s.stateImage.ImageScratchSize = s.stateScratchUsed
	}
	return address
}

// placeStatic checks a static place and returns it as an operand. A stack
// offset is relative to the frame and must be inside the frame of the
// current function. The old loop lets a larger one run into the data
// segment. An absolute place must be inside the data segment.
func (s *state) placeStatic(p place) (operand, ReasonCode) {
	lowered := operand{operandOffset: p.placeOffset, operandFrame: p.placeFrame, operandSize: p.placeSize}
	if p.placeFrame && uint64(p.placeOffset)+uint64(p.placeSize) > uint64(s.stateFrameSize) {
		return operand{}, ReasonOperandOutsideFrame
	}
	if !p.placeFrame && (p.placeOffset < s.stateDataStart || uint64(p.placeOffset)+uint64(p.placeSize) > uint64(s.stateDataEnd)) {
		return operand{}, ReasonOperandOutsideData
	}
	return lowered, ReasonNone
}

// placeOf lowers the address computation of an operand, the steps
// GetFinalOffset takes for its type signature. Each index, dereference and
// field becomes code. A place without index or dereference stays static,
// with its field offsets added.
func (s *state) placeOf(code []ir.Instr, signatureIndex ast.CXTypeSignatureIndex) ([]ir.Instr, place, ReasonCode) {
	prgrm := s.statePrgrm
	signature := prgrm.GetCXTypeSignatureFromArray(signatureIndex)
	size := pointerToUint32(signature.GetSize(prgrm, false))
	if ast.OffsetEscaped(signature.GetOffset(prgrm)) {
		return code, place{}, ReasonOperandHeap
	}
	var reason ReasonCode
	switch signature.Type {
	case ast.TYPE_ATOMIC:
		p := s.placeBase(signature.Offset, size)
		return code, p, ReasonNone
	case ast.TYPE_POINTER_ATOMIC:
		if signature.IsDeref {
			return code, place{}, ReasonOperandIndexed
		}
		return code, s.placeBase(signature.Offset, size), ReasonNone
	case ast.TYPE_ARRAY_ATOMIC, ast.TYPE_POINTER_ARRAY_ATOMIC:
		p := s.placeBase(signature.Offset, size)
		if signature.Type == ast.TYPE_POINTER_ARRAY_ATOMIC {
			if !signature.IsDeref {
				return code, p, ReasonNone
			}
			if code, reason = s.placeDeref(code, &p); reason != ReasonNone {
				return code, p, reason
			}
		}
		details := prgrm.GetCXTypeSignatureArrayFromArray(signature.Meta)
		elementSize := pointerToUint32(types.Code(details.Meta).Size())
		for i, indexSignature := range details.Indexes {
			if i >= len(details.Lengths) {
				return code, p, ReasonOperandIndexed
			}
			stride := pointerToUint32(types.TotalLength(details.Lengths[i+1:])) * elementSize
			if code, reason = s.placeIndex(code, &p, indexSignature, stride); reason != ReasonNone {
				return code, p, reason
			}
		}
		return code, p, ReasonNone
	case ast.TYPE_SLICE_ATOMIC:
		if len(prgrm.GetCXTypeSignatureArrayFromArray(signature.Meta).Indexes) > 0 {
			return code, place{}, ReasonOperandHeap
		}
		return code, s.placeBase(signature.Offset, size), ReasonNone
	case ast.TYPE_POINTER_SLICE_ATOMIC:
		if signature.IsDeref {
			return code, place{}, ReasonOperandHeap
		}
		return code, s.placeBase(signature.Offset, size), ReasonNone
	case ast.TYPE_STRUCT, ast.TYPE_POINTER_STRUCT:
		p := s.placeBase(signature.Offset, size)
		if signature.Type == ast.TYPE_POINTER_STRUCT && signature.IsDeref {
			if code, reason = s.placeDeref(code, &p); reason != ReasonNone {
				return code, p, reason
			}
		}
		for _, fieldIndex := range prgrm.GetCXTypeSignatureStructFromArray(signature.Meta).Fields {
			field := prgrm.GetCXArgFromArray(fieldIndex)
			placeAdd(&p, pointerToUint32(field.Offset))
			if code, reason = s.placeDereferences(code, &p, field); reason != ReasonNone {
				return code, p, reason
			}
		}
		return code, p, ReasonNone
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		arg := prgrm.GetCXArg(ast.CXArgumentIndex(signature.Meta))
		p := s.placeBase(arg.Offset, size)
		if code, reason = s.placeDereferences(code, &p, arg); reason != ReasonNone {
			return code, p, reason
		}
		for _, fieldIndex := range arg.Fields {
			field := prgrm.GetCXArgFromArray(fieldIndex)
			placeAdd(&p, pointerToUint32(field.Offset))
			if code, reason = s.placeDereferences(code, &p, field); reason != ReasonNone {
				return code, p, reason
			}
		}
		return code, p, ReasonNone
	}
	return code, place{}, ReasonOperandNotScalar
}

// placeBase is a static place at offset, relative to the frame when offset
// is inside the stack.
func (s *state) placeBase(offset types.Pointer, size uint32) place {
	base := pointerToUint32(offset)
	return place{placeOffset: base, placeFrame: base < s.stateStackSize, placeSize: size}
}

// placeAdd adds a field offset to p.
func placeAdd(p *place, offset uint32) {
	if p.placeComputed {
		p.placeConstant += offset
	} else {
		p.placeOffset += offset
	}
}

// placeDereferences follows the DereferenceOperations of arg as
// CalculateDereferences does. Array and slice steps are skipped when arg has
// no indexes.
func (s *state) placeDereferences(code []ir.Instr, p *place, arg *ast.CXArgument) ([]ir.Instr, ReasonCode) {
	indexCount := 0
	derefPointer := false
	var reason ReasonCode
	for _, operation := range arg.DereferenceOperations {
		if len(arg.Indexes) == 0 && operation != constants.DEREF_POINTER {
			continue
		}
		switch operation {
		case constants.DEREF_SLICE:
			return code, ReasonOperandHeap
		case constants.DEREF_ARRAY:
			if indexCount >= len(arg.Indexes) || indexCount >= len(arg.Lengths) {
				return code, ReasonOperandIndexed
			}
			elementSize := ast.GetDerefSize(s.statePrgrm, arg, indexCount, derefPointer, true)
			lengths := arg.Lengths[indexCount+1:]
			if direct, behind := arg.DirectDimensions(); behind && indexCount < direct {
				lengths = arg.Lengths[indexCount+1 : direct]
			} else if dims := arg.ArraySliceDimensions(); dims > indexCount {
				// The elements of [N][]T are slice headers.
				elementSize, lengths = types.POINTER_SIZE, arg.Lengths[indexCount+1:dims]
			}
			stride := pointerToUint32(types.TotalLength(lengths) * elementSize)
			derefPointer = false
			if code, reason = s.placeIndex(code, p, arg.Indexes[indexCount], stride); reason != ReasonNone {
				return code, reason
			}
			indexCount++
		case constants.DEREF_POINTER:
			if code, reason = s.placeDeref(code, p); reason != ReasonNone {
				return code, reason
			}
			derefPointer = true
		}
	}
	return code, ReasonNone
}

// placeIndex adds the i32 at indexSignature times stride to p, which
// becomes computed. The old loop reads 4 bytes at the index whatever its
// type.
func (s *state) placeIndex(code []ir.Instr, p *place, indexSignature ast.CXTypeSignatureIndex, stride uint32) ([]ir.Instr, ReasonCode) {
	code, indexPlace, reason := s.placeOf(code, indexSignature)
	if reason != ReasonNone {
		return code, reason
	}
	indexPlace.placeSize = 4
	code, index, reason := s.placeRead(code, indexPlace)
	if reason != ReasonNone {
		return code, reason
	}
	in := ir.Instr{InstrN: stride}
	operandSet(&in, 1, index)
	if p.placeComputed {
		in.InstrOpcode = ir.OpcodeAddressIndex
		in.InstrA = p.placeOffset
		in.InstrM = p.placeConstant
		in.InstrC = p.placeOffset
	} else {
		base, reason := s.placeStatic(place{placeOffset: p.placeOffset, placeFrame: p.placeFrame})
		if reason != ReasonNone {
			return code, reason
		}
		in.InstrOpcode = ir.OpcodeAddressIndexStatic
		operandSet(&in, 0, base)
		in.InstrC = s.scratch(4)
	}
	*p = place{placeOffset: in.InstrC, placeComputed: true, placeSize: p.placeSize}
	return append(code, in), ReasonNone
}

// placeDeref replaces p by the pointer stored at p.
func (s *state) placeDeref(code []ir.Instr, p *place) ([]ir.Instr, ReasonCode) {
	var in ir.Instr
	if p.placeComputed {
		in = ir.Instr{InstrOpcode: ir.OpcodeAddressDeref, InstrA: p.placeOffset, InstrM: p.placeConstant, InstrC: p.placeOffset}
	} else {
		pointer, reason := s.placeStatic(place{placeOffset: p.placeOffset, placeFrame: p.placeFrame, placeSize: machinePointerSize})
		if reason != ReasonNone {
			return code, reason
		}
		in = ir.Instr{InstrOpcode: ir.OpcodeAddressDerefStatic, InstrC: s.scratch(4)}
		operandSet(&in, 0, pointer)
	}
	*p = place{placeOffset: in.InstrC, placeComputed: true, placeSize: p.placeSize}
	return append(code, in), ReasonNone
}

// operandSet puts lowered in operand slot i of in, 0 to 3 for A to D.
func operandSet(in *ir.Instr, i int, lowered operand) {
	var mode ir.InstrMode
	switch i {
	case 0:
		in.InstrA = lowered.operandOffset
		mode = ir.InstrModeFrameA
	case 1:
		in.InstrB = lowered.operandOffset
		mode = ir.InstrModeFrameB
	case 2:
		in.InstrC = lowered.operandOffset
		mode = ir.InstrModeFrameC
	case 3:
		in.InstrD = lowered.operandOffset
		mode = ir.InstrModeFrameD
	default:
		assert.AssertUnreachable(fmt.Sprintf("lower: operand slot %d", i))
	}
	if lowered.operandFrame {
		in.InstrMode |= mode
	}
}

// file returns the index of the path of file in ImageFileNames, adding it
// when new.
func (s *state) file(file ast.SourceFileGid) uint32 {
	if index, found := s.stateFiles[file]; found {
		return index
	}
	index := uint32(len(s.stateImage.ImageFileNames))
	s.stateImage.ImageFileNames = append(s.stateImage.ImageFileNames, s.statePrgrm.CXProgramSourceFileText(file))
	s.stateFiles[file] = index
	return index
}

// pointerToUint32 narrows an offset or size. An image addresses 4 GB.
func pointerToUint32(pointer types.Pointer) uint32 {
	assert.AssertInvariantTrue(uint64(pointer) <= uint64(^uint32(0)), "lower: offset does not fit in 32 bits")
	return uint32(pointer)
}

// libraryBufferInput reports whether a library function input is a byte
// buffer, an input of undefined type. The caller passes a ui8 array, which
// the function reads or fills in place, see docs/Networking.md.
func libraryBufferInput(prgrm *ast.CXProgram, signatureIndex ast.CXTypeSignatureIndex) bool {
	signature := prgrm.GetCXTypeSignatureFromArray(signatureIndex)
	switch signature.Type {
	case ast.TYPE_ATOMIC:
		return types.Code(signature.Meta) == types.UNDEFINED
	case ast.TYPE_CXARGUMENT_DEPRECATE:
		return prgrm.GetCXArgFromArray(ast.CXArgumentIndex(signature.Meta)).Type == types.UNDEFINED
	}
	return false
}

// libraryBuffer lowers a byte buffer input as the address and the byte size
// of the ui8 array, each a pointer, at offset of the block. The vm form
// reads or fills memory there, as the old loop does at the array's offset.
func (s *state) libraryBuffer(code []ir.Instr, signatureIndex ast.CXTypeSignatureIndex, offset uint32) ([]ir.Instr, ReasonCode) {
	prgrm := s.statePrgrm
	signature := prgrm.GetCXTypeSignatureFromArray(signatureIndex)
	if signature.Type != ast.TYPE_ARRAY_ATOMIC || signature.GetType(prgrm) != types.UI8 {
		return code, ReasonOperandNotScalar
	}
	code, p, reason := s.placeOf(code, signatureIndex)
	if reason != ReasonNone {
		return code, reason
	}
	code, address, reason := s.addressOf(code, p)
	if reason != ReasonNone {
		return code, reason
	}
	addressSlot := operand{operandOffset: s.scratch(machinePointerSize), operandSize: machinePointerSize}
	operandSet(&address, 2, addressSlot)
	// OpcodeAddressOf of an absolute A writes A itself, here the size.
	sizeSlot := operand{operandOffset: s.scratch(machinePointerSize), operandSize: machinePointerSize}
	size := ir.Instr{InstrOpcode: ir.OpcodeAddressOf, InstrA: p.placeSize}
	operandSet(&size, 2, sizeSlot)
	addressArg := ir.Instr{InstrOpcode: ir.OpcodeCallArg, InstrC: offset, InstrN: machinePointerSize}
	operandSet(&addressArg, 0, addressSlot)
	sizeArg := ir.Instr{InstrOpcode: ir.OpcodeCallArg, InstrC: offset + machinePointerSize, InstrN: machinePointerSize}
	operandSet(&sizeArg, 0, sizeSlot)
	return append(code, address, size, addressArg, sizeArg), ReasonNone
}

// libraryCall lowers a call of a system library function that has a vm form,
// as call lowers a call: a block at sp holds the inputs and then the
// outputs at the offsets of the native function, ir.OpcodeLibraryCall runs
// the function on it, and the outputs are copied out.
func (s *state) libraryCall(code []ir.Instr, cxAtomicOp *ast.CXAtomicOperator, callee *ast.CXFunction) ([]ir.Instr, ReasonCode) {
	if !vm.LibraryFunctionVmLinked(callee.LibraryNumber, callee.LibraryFunctionNumber) {
		return code, ReasonLibraryFunctionOutsideSubset
	}
	prgrm := s.statePrgrm
	inputs := cxAtomicOp.GetInputs(prgrm)
	outputs := cxAtomicOp.GetOutputs(prgrm)
	calleeInputs := callee.GetInputs(prgrm)
	calleeOutputs := callee.GetOutputs(prgrm)
	if len(inputs) != len(calleeInputs) || len(outputs) > len(calleeOutputs) {
		return code, ReasonOperandCount
	}
	const blockLimit = ^uint32(0)
	// A byte buffer input has no size of its own, so it takes the two
	// pointers libraryBuffer writes.
	inputSlots := make([]ir.Slot, len(calleeInputs))
	for i, signatureIndex := range calleeInputs {
		if libraryBufferInput(prgrm, signatureIndex) {
			inputSlots[i].SlotSize = 2 * machinePointerSize
			continue
		}
		slot, reason := s.slot(signatureIndex, blockLimit)
		if reason != ReasonNone {
			return code, ReasonCalleeLayout
		}
		inputSlots[i] = slot
	}
	outputSlots, reason := s.slots(calleeOutputs, blockLimit)
	if reason != ReasonNone {
		return code, ReasonCalleeLayout
	}
	// The signatures give the sizes. The block packs the inputs and then the
	// outputs in order, the layout the vm form of the function reads.
	blockSize := uint32(0)
	for _, slots := range [][]ir.Slot{inputSlots, outputSlots} {
		for i := range slots {
			slots[i].SlotOffset = blockSize
			blockSize += slots[i].SlotSize
		}
	}

	code = append(code, ir.Instr{InstrOpcode: ir.OpcodeCallFrame, InstrN: blockSize})
	for i, signatureIndex := range inputs {
		if libraryBufferInput(prgrm, calleeInputs[i]) {
			if code, reason = s.libraryBuffer(code, signatureIndex, inputSlots[i].SlotOffset); reason != ReasonNone {
				return code, reason
			}
			continue
		}
		if ast.CXTypeSignaturePassBy(prgrm, prgrm.GetCXTypeSignatureFromArray(signatureIndex)) == constants.PASSBY_REFERENCE {
			return code, ReasonOperandByReference
		}
		var value operand
		if code, value, reason = s.read(code, signatureIndex); reason != ReasonNone {
			return code, reason
		}
		if value.operandSize != inputSlots[i].SlotSize {
			return code, ReasonOperandCount
		}
		in := ir.Instr{InstrOpcode: ir.OpcodeCallArg, InstrC: inputSlots[i].SlotOffset, InstrN: value.operandSize}
		operandSet(&in, 0, value)
		code = append(code, in)
	}
	code = append(code, ir.Instr{InstrOpcode: ir.OpcodeLibraryCall, InstrM: uint32(callee.LibraryNumber), InstrN: uint32(callee.LibraryFunctionNumber)})
	return s.callResults(code, outputs, outputSlots[:len(outputs)])
}
