package ast

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
)

// LibraryFunctionHandlers holds the handler of each function of each system
// library, indexed by library number and then function number. A library
// that is not linked into this binary has no handlers.
// VIOLATION: state outside State. Owner: t3-ast; moves to CXNativeFunctionMng in phase 1.
var LibraryFunctionHandlers [constants.LibraryCount][]OpcodeHandler

// libraryFunctionKeyShift leaves room for 65535 functions per library.
const libraryFunctionKeyShift = 16

// LibraryFunctionKey returns the key of a library function in NativeNames
// and Natives. It is negative, so it never meets an opcode number.
func LibraryFunctionKey(library int, function int) int {
	return -(library<<libraryFunctionKeyShift | function)
}

// LibraryLinked reports whether library has handlers in this binary.
func LibraryLinked(library int) bool {
	return len(LibraryFunctionHandlers[library]) > 0
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
	return NativeNames.NativeNameIndexCodeText(fn.AtomicOPCode)
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

// libraryCall runs library function fn after the sandbox check, one indexed
// load and compare.
// VIOLATION STRHOT-287: E, error or assert text, built on a failure branch only, #412.
func libraryCall(prgrm *CXProgram, fn *CXFunction, inputs []CXValue, outputs []CXValue) {
	if !prgrm.ProgramLibrariesEnabled[fn.LibraryNumber] {
		panic(RuntimeLibraryDisabled(RuntimeLibraryDisabledMessage(fn.LibraryNumber)))
	}
	LibraryFunctionHandlers[fn.LibraryNumber][fn.LibraryFunctionNumber](prgrm, inputs, outputs)
}

// LibraryRunStarts holds what each linked library does before a run that
// enables it, such as the net library starting its sockets and replay
// record, nil for nothing. An error stops the run before it starts.
// VIOLATION: state outside State. Owner: t3-ast; moves to CXNativeFunctionMng in phase 1.
var LibraryRunStarts [constants.LibraryCount]func() error
