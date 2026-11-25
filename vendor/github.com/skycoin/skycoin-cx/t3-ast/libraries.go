package ast

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// LibraryDispatchFunction runs function of one system library on its
// operands and reports false for a number the library does not define. The
// switches are generated from constants.LibraryFunctionDefinitionTable
// (decision D96).
type LibraryDispatchFunction func(prgrm *CXProgram, function int, inputs []CXValue, outputs []CXValue) bool

// LibraryDispatches holds the switch of each system library this binary
// links, by library number, nil for a library it leaves out. The
// registration of the binary sets it, see opcodes.LibraryLink.
// VIOLATION: state outside State. Owner: t3-ast; moves to CXNativeFunctionMng in phase 1.
var LibraryDispatches [constants.LibraryCount]LibraryDispatchFunction

// LibraryFunctionKey returns the key of a library function in NativeNames
// and NativeFunctionOf. It is negative, so it never meets an opcode number.
func LibraryFunctionKey(library int, function int) int {
	return constants.LibraryFunctionKey(library, function)
}

// LibraryLinked reports whether this binary links library.
func LibraryLinked(library int) bool {
	return library > 0 && library < constants.LibraryCount && LibraryDispatches[library] != nil
}

// LibraryFunctionLinked reports whether this binary links library and the
// library defines function.
func LibraryFunctionLinked(library int, function int) bool {
	return LibraryLinked(library) && constants.LibraryFunctionSignatureOf(library, function).LibraryFunctionSignatureDefined
}

// BuiltInName returns the name of the built-in fn calls, the library function
// for constants.OP_LIBRARY_CALL and the opcode otherwise.
// VIOLATION STRHOT-286: B, name text for compiler code run by an opcode, #412.
func (fn *CXFunction) BuiltInName() string {
	if fn.AtomicOPCode == constants.OP_LIBRARY_CALL {
		// A binary that does not link the library, such as a cx build core,
		// has no NativeNames entry for it.
		row, _ := constants.LibraryFunctionLiveByNumber(fn.LibraryNumber, fn.LibraryFunctionNumber)
		return row.Name
	}
	return constants.OpcodeNameOf(fn.AtomicOPCode)
}

// BuiltInNameBytes returns the bytes of BuiltInName without building a
// string, for printing on the run path. The caller must not write to them.
func (fn *CXFunction) BuiltInNameBytes() []byte {
	if fn.AtomicOPCode == constants.OP_LIBRARY_CALL {
		return constants.LibraryFunctionLiveNameBytes(fn.LibraryNumber, fn.LibraryFunctionNumber)
	}
	return constants.OpcodeNameBytesOf(fn.AtomicOPCode)
}

// RuntimeLibraryDisabled is the panic value of a call into a system library
// the run did not enable. It is the message the runtime error prints.
type RuntimeLibraryDisabled string

// RuntimeLibraryDisabledMessage is the message of a call into the disabled
// library number.
// VIOLATION STRHOT-288: E, error or assert text, built on a failure branch only, #412.
func RuntimeLibraryDisabledMessage(library int) string {
	name := constants.LibraryNameOf(library)
	return "library " + name + " is not enabled, enable it with --enable=" + name
}

// libraryCall runs library function fn through the switch of its library
// after the sandbox check, one indexed load and compare.
// VIOLATION STRHOT-287: E, error or assert text, built on a failure branch only, #412.
func libraryCall(prgrm *CXProgram, fn *CXFunction, inputs []CXValue, outputs []CXValue) {
	if !prgrm.ProgramLibrariesEnabled[fn.LibraryNumber] {
		panic(RuntimeLibraryDisabled(RuntimeLibraryDisabledMessage(fn.LibraryNumber)))
	}
	dispatch := LibraryDispatches[fn.LibraryNumber]
	if dispatch == nil || !dispatch(prgrm, fn.LibraryFunctionNumber, inputs, outputs) {
		panic(RuntimeOpcodeUnlinked(constants.OP_LIBRARY_CALL))
	}
}

// LibraryRunStarts holds what each linked library does before a run that
// enables it, such as the net library starting its sockets and replay
// record, nil for nothing. An error stops the run before it starts.
// VIOLATION: state outside State. Owner: t3-ast; moves to CXNativeFunctionMng in phase 1.
var LibraryRunStarts [constants.LibraryCount]func() error
