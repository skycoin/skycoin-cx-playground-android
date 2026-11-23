// Package ir holds the executable image of a CX program, the flat form the
// run time executes, see docs/IR.md and skycoin/skycoin-cx#408. An Image is
// plain data. It holds no pointer to an IR object, only indexes and offsets,
// and no string except its line and name tables.
package ir

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// Instr is one instruction, a fixed 32 byte record. A, B and C are operand
// byte offsets, D is a fourth operand for natives with more inputs. N and M
// are immediates, a size, a jump target or a function id. Aux indexes a side
// table. The meaning of each field per opcode is in the Opcode list.
type Instr struct {
	InstrOpcode Opcode
	InstrMode   InstrMode
	InstrA      uint32
	InstrB      uint32
	InstrC      uint32
	InstrD      uint32
	InstrN      uint32
	InstrM      uint32
	InstrAux    uint32
}

// InstrSize is the size of an Instr in bytes, checked by a test.
const InstrSize = 32

// InstrMode holds one bit per operand. A set bit makes the operand relative
// to the frame pointer, a clear bit makes it an absolute address.
type InstrMode uint16

const (
	InstrModeFrameA InstrMode = 1 << 0
	InstrModeFrameB InstrMode = 1 << 1
	InstrModeFrameC InstrMode = 1 << 2
	InstrModeFrameD InstrMode = 1 << 3
)

// Slot is the byte layout of one function input or output in its frame.
type Slot struct {
	SlotOffset uint32
	SlotSize   uint32
}

// Line maps the instructions from LinePc up to the next Line of the same
// function to a source line. LineFile indexes ImageFileNames.
type Line struct {
	LinePc     uint32
	LineFile   uint32
	LineNumber uint32
}

// Func is one function. FuncCode is fixed once the image is built and ends
// with OpcodeReturn. A function that was not lowered has no code. Its lines
// are ImageLines[FuncLineFirst : FuncLineFirst+FuncLineCount], in pc order.
type Func struct {
	FuncCode      []Instr
	FuncFrameSize uint32
	FuncInputs    []Slot
	FuncOutputs   []Slot
	FuncLineFirst uint32
	FuncLineCount uint32
}

// Image is a program ready to run. ImageFuncs is indexed like the compiler's
// function list, so a function id is the same in both. Memory is the stack,
// ImageStackSize bytes from 0, then ImageData at ImageDataStart, then
// ImageScratchSize bytes of scratch that only the machine sees. The scratch
// holds computed addresses and loaded values for one expression at a time.
// ImageText holds the constant bytes OpcodePrintText writes. An entry is a
// function id or ImageFuncNone. ImageFuncNames[i] names ImageFuncs[i].
type Image struct {
	ImageFuncs       []Func
	ImageData        []byte
	ImageDataStart   uint32
	ImageStackSize   uint32
	ImageScratchSize uint32
	ImageText        []byte
	ImageEntryInit   uint32
	ImageEntryMain   uint32
	ImageLines       []Line
	ImageFileNames   []string
	ImageFuncNames   []string
}

// ImageDataEnd returns the first byte after the data segment, where the
// scratch starts.
func ImageDataEnd(image *Image) uint32 {
	return image.ImageDataStart + uint32(len(image.ImageData))
}

// PrintStrMode is how OpcodePrintStr writes a str.
type PrintStrMode uint32

const (
	// PrintStrModeRaw writes the bytes as they are, printf %v.
	PrintStrModeRaw PrintStrMode = iota
	// PrintStrModeEscaped turns \% into % and \n into a new line, printf %s
	// as CheckForEscapedChars does.
	PrintStrModeEscaped
	// PrintStrModeLine writes the bytes and a new line, string.PrintLine.
	PrintStrModeLine
)

// ImageFuncNone is the entry id of a missing entry function.
const ImageFuncNone = ^uint32(0)

// ImageCallDepthMax is the deepest call nesting an image may reach, the size
// of the fixed frame array of a machine.
const ImageCallDepthMax = uint32(constants.CALLSTACK_SIZE)

// LineAt returns the line of pc in function, false when it has none.
func LineAt(image *Image, function uint32, pc uint32) (Line, bool) {
	fn := &image.ImageFuncs[function]
	lines := image.ImageLines[fn.FuncLineFirst : fn.FuncLineFirst+fn.FuncLineCount]
	found := false
	var line Line
	for i := range lines {
		if lines[i].LinePc > pc {
			break
		}
		line = lines[i]
		found = true
	}
	return line, found
}

// Opcode is an image opcode. The numbers are dense and grouped by family so
// a switch over them compiles to a jump table. They are not the language
// opcode numbers of t1-constants, which are sparse, fixed for serialized
// programs and have no number for moves, calls or returns. Opcodes exist only
// inside an image and are renumbered freely.
//
// An operand X is the byte offset InstrX, plus the frame pointer when its
// mode bit is set. sp is the first byte after the current frame.
type Opcode uint16

const (
	// OpcodeInvalid is the zero Instr, never valid.
	OpcodeInvalid Opcode = iota
	// OpcodeReturn returns to the caller, or ends the run at the entry.
	OpcodeReturn
	// OpcodeJump continues at pc M.
	OpcodeJump
	// OpcodeJumpIf continues at pc M when the bool at A is true, else at N.
	OpcodeJumpIf
	// OpcodeZero writes N zero bytes at A.
	OpcodeZero
	// OpcodeMove copies N bytes from A to C.
	OpcodeMove
	// OpcodeCallFrame writes N zero bytes at sp, the frame of the next call.
	OpcodeCallFrame
	// OpcodeCallArg copies N bytes from A to sp+C, an input of the next call.
	OpcodeCallArg
	// OpcodeCall calls function M with its frame at sp.
	OpcodeCall
	// OpcodeCallResult copies N bytes from sp+A, an output of the call that
	// just returned, to C.
	OpcodeCallResult
	// OpcodeTestEqual compares N bytes at A with M bytes at B and counts a
	// failed test when they differ. C is the message string.
	OpcodeTestEqual
	// OpcodeAddressIndexStatic writes the address A plus N times the i32 at
	// B to the address slot C. A negative index stops the run. An address
	// slot is a uint32 in the scratch area.
	OpcodeAddressIndexStatic
	// OpcodeAddressIndex writes the address in slot A plus M, plus N times
	// the i32 at B, to slot C.
	OpcodeAddressIndex
	// OpcodeAddressDerefStatic writes the pointer at A to slot C. A pointer
	// outside the stack and data segment stops the run.
	OpcodeAddressDerefStatic
	// OpcodeAddressDeref writes the pointer at the address in slot A plus M
	// to slot C, checked as OpcodeAddressDerefStatic.
	OpcodeAddressDeref
	// OpcodeAddressLoad copies N bytes from the address in slot A plus M to
	// C.
	OpcodeAddressLoad
	// OpcodeAddressStore copies N bytes from A to the address in slot C plus
	// M. D is 1, so the loop tells it from OpcodeAddressLoad without
	// reading the opcode again.
	OpcodeAddressStore
	// OpcodeAddressOf writes the address A as a pointer at C.
	OpcodeAddressOf
	// OpcodeAddressPointer writes the address in slot A plus M as a pointer
	// at C.
	OpcodeAddressPointer
	// OpcodePrintText writes M bytes of ImageText from N to the output.
	OpcodePrintText
	// OpcodePrintValue writes the value at A, of types.Code M, to the
	// output in the printf verb N, one of d, f, b and v.
	OpcodePrintValue
	// OpcodePrintStr writes the bytes of the str at A to the output. N is a
	// PrintStrMode.
	OpcodePrintStr
	// OpcodeBoolNot and the rest of the bool family read A and B and write C.
	OpcodeBoolNot
	OpcodeBoolAnd
	OpcodeBoolOr
	OpcodeBoolEq
	OpcodeBoolUneq
	OpcodeBoolPrint

	// The scalar families read A and B and write C. Print writes A and a new
	// line to the output. Each family keeps the order of its language names.
	OpcodeI8Add
	OpcodeI8Sub
	OpcodeI8Mul
	OpcodeI8Div
	OpcodeI8Mod
	OpcodeI8Neg
	OpcodeI8Abs
	OpcodeI8Max
	OpcodeI8Min
	OpcodeI8Eq
	OpcodeI8Uneq
	OpcodeI8Lt
	OpcodeI8Lteq
	OpcodeI8Gt
	OpcodeI8Gteq
	OpcodeI8BitAnd
	OpcodeI8BitOr
	OpcodeI8BitXor
	OpcodeI8BitClear
	OpcodeI8BitShl
	OpcodeI8BitShr
	OpcodeI8Print
	OpcodeI16Add
	OpcodeI16Sub
	OpcodeI16Mul
	OpcodeI16Div
	OpcodeI16Mod
	OpcodeI16Neg
	OpcodeI16Abs
	OpcodeI16Max
	OpcodeI16Min
	OpcodeI16Eq
	OpcodeI16Uneq
	OpcodeI16Lt
	OpcodeI16Lteq
	OpcodeI16Gt
	OpcodeI16Gteq
	OpcodeI16BitAnd
	OpcodeI16BitOr
	OpcodeI16BitXor
	OpcodeI16BitClear
	OpcodeI16BitShl
	OpcodeI16BitShr
	OpcodeI16Print
	OpcodeI32Add
	OpcodeI32Sub
	OpcodeI32Mul
	OpcodeI32Div
	OpcodeI32Mod
	OpcodeI32Neg
	OpcodeI32Abs
	OpcodeI32Max
	OpcodeI32Min
	OpcodeI32Eq
	OpcodeI32Uneq
	OpcodeI32Lt
	OpcodeI32Lteq
	OpcodeI32Gt
	OpcodeI32Gteq
	OpcodeI32BitAnd
	OpcodeI32BitOr
	OpcodeI32BitXor
	OpcodeI32BitClear
	OpcodeI32BitShl
	OpcodeI32BitShr
	OpcodeI32Print
	OpcodeI64Add
	OpcodeI64Sub
	OpcodeI64Mul
	OpcodeI64Div
	OpcodeI64Mod
	OpcodeI64Neg
	OpcodeI64Abs
	OpcodeI64Max
	OpcodeI64Min
	OpcodeI64Eq
	OpcodeI64Uneq
	OpcodeI64Lt
	OpcodeI64Lteq
	OpcodeI64Gt
	OpcodeI64Gteq
	OpcodeI64BitAnd
	OpcodeI64BitOr
	OpcodeI64BitXor
	OpcodeI64BitClear
	OpcodeI64BitShl
	OpcodeI64BitShr
	OpcodeI64Print
	OpcodeUI8Add
	OpcodeUI8Sub
	OpcodeUI8Mul
	OpcodeUI8Div
	OpcodeUI8Mod
	OpcodeUI8Max
	OpcodeUI8Min
	OpcodeUI8Eq
	OpcodeUI8Uneq
	OpcodeUI8Lt
	OpcodeUI8Lteq
	OpcodeUI8Gt
	OpcodeUI8Gteq
	OpcodeUI8BitAnd
	OpcodeUI8BitOr
	OpcodeUI8BitXor
	OpcodeUI8BitClear
	OpcodeUI8BitShl
	OpcodeUI8BitShr
	OpcodeUI8Print
	OpcodeUI16Add
	OpcodeUI16Sub
	OpcodeUI16Mul
	OpcodeUI16Div
	OpcodeUI16Mod
	OpcodeUI16Max
	OpcodeUI16Min
	OpcodeUI16Eq
	OpcodeUI16Uneq
	OpcodeUI16Lt
	OpcodeUI16Lteq
	OpcodeUI16Gt
	OpcodeUI16Gteq
	OpcodeUI16BitAnd
	OpcodeUI16BitOr
	OpcodeUI16BitXor
	OpcodeUI16BitClear
	OpcodeUI16BitShl
	OpcodeUI16BitShr
	OpcodeUI16Print
	OpcodeUI32Add
	OpcodeUI32Sub
	OpcodeUI32Mul
	OpcodeUI32Div
	OpcodeUI32Mod
	OpcodeUI32Max
	OpcodeUI32Min
	OpcodeUI32Eq
	OpcodeUI32Uneq
	OpcodeUI32Lt
	OpcodeUI32Lteq
	OpcodeUI32Gt
	OpcodeUI32Gteq
	OpcodeUI32BitAnd
	OpcodeUI32BitOr
	OpcodeUI32BitXor
	OpcodeUI32BitClear
	OpcodeUI32BitShl
	OpcodeUI32BitShr
	OpcodeUI32Print
	OpcodeUI64Add
	OpcodeUI64Sub
	OpcodeUI64Mul
	OpcodeUI64Div
	OpcodeUI64Mod
	OpcodeUI64Max
	OpcodeUI64Min
	OpcodeUI64Eq
	OpcodeUI64Uneq
	OpcodeUI64Lt
	OpcodeUI64Lteq
	OpcodeUI64Gt
	OpcodeUI64Gteq
	OpcodeUI64BitAnd
	OpcodeUI64BitOr
	OpcodeUI64BitXor
	OpcodeUI64BitClear
	OpcodeUI64BitShl
	OpcodeUI64BitShr
	OpcodeUI64Print
	OpcodeF32Add
	OpcodeF32Sub
	OpcodeF32Mul
	OpcodeF32Div
	OpcodeF32Mod
	OpcodeF32Neg
	OpcodeF32Abs
	OpcodeF32Max
	OpcodeF32Min
	OpcodeF32Eq
	OpcodeF32Uneq
	OpcodeF32Lt
	OpcodeF32Lteq
	OpcodeF32Gt
	OpcodeF32Gteq
	OpcodeF32Print
	OpcodeF64Add
	OpcodeF64Sub
	OpcodeF64Mul
	OpcodeF64Div
	OpcodeF64Mod
	OpcodeF64Neg
	OpcodeF64Abs
	OpcodeF64Max
	OpcodeF64Min
	OpcodeF64Eq
	OpcodeF64Uneq
	OpcodeF64Lt
	OpcodeF64Lteq
	OpcodeF64Gt
	OpcodeF64Gteq
	OpcodeF64Print
	OpcodeI8ToI16
	OpcodeI8ToI32
	OpcodeI8ToI64
	OpcodeI8ToUI8
	OpcodeI8ToUI16
	OpcodeI8ToUI32
	OpcodeI8ToUI64
	OpcodeI8ToF32
	OpcodeI8ToF64
	OpcodeI16ToI8
	OpcodeI16ToI32
	OpcodeI16ToI64
	OpcodeI16ToUI8
	OpcodeI16ToUI16
	OpcodeI16ToUI32
	OpcodeI16ToUI64
	OpcodeI16ToF32
	OpcodeI16ToF64
	OpcodeI32ToI8
	OpcodeI32ToI16
	OpcodeI32ToI64
	OpcodeI32ToUI8
	OpcodeI32ToUI16
	OpcodeI32ToUI32
	OpcodeI32ToUI64
	OpcodeI32ToF32
	OpcodeI32ToF64
	OpcodeI64ToI8
	OpcodeI64ToI16
	OpcodeI64ToI32
	OpcodeI64ToUI8
	OpcodeI64ToUI16
	OpcodeI64ToUI32
	OpcodeI64ToUI64
	OpcodeI64ToF32
	OpcodeI64ToF64
	OpcodeUI8ToI8
	OpcodeUI8ToI16
	OpcodeUI8ToI32
	OpcodeUI8ToI64
	OpcodeUI8ToUI16
	OpcodeUI8ToUI32
	OpcodeUI8ToUI64
	OpcodeUI8ToF32
	OpcodeUI8ToF64
	OpcodeUI16ToI8
	OpcodeUI16ToI16
	OpcodeUI16ToI32
	OpcodeUI16ToI64
	OpcodeUI16ToUI8
	OpcodeUI16ToUI32
	OpcodeUI16ToUI64
	OpcodeUI16ToF32
	OpcodeUI16ToF64
	OpcodeUI32ToI8
	OpcodeUI32ToI16
	OpcodeUI32ToI32
	OpcodeUI32ToI64
	OpcodeUI32ToUI8
	OpcodeUI32ToUI16
	OpcodeUI32ToUI64
	OpcodeUI32ToF32
	OpcodeUI32ToF64
	OpcodeUI64ToI8
	OpcodeUI64ToI16
	OpcodeUI64ToI32
	OpcodeUI64ToI64
	OpcodeUI64ToUI8
	OpcodeUI64ToUI16
	OpcodeUI64ToUI32
	OpcodeUI64ToF32
	OpcodeUI64ToF64
	OpcodeF32ToI8
	OpcodeF32ToI16
	OpcodeF32ToI32
	OpcodeF32ToI64
	OpcodeF32ToUI8
	OpcodeF32ToUI16
	OpcodeF32ToUI32
	OpcodeF32ToUI64
	OpcodeF32ToF64
	OpcodeF64ToI8
	OpcodeF64ToI16
	OpcodeF64ToI32
	OpcodeF64ToI64
	OpcodeF64ToUI8
	OpcodeF64ToUI16
	OpcodeF64ToUI32
	OpcodeF64ToUI64
	OpcodeF64ToF32

	// OpcodeCount is one past the last opcode.
	OpcodeCount
)
