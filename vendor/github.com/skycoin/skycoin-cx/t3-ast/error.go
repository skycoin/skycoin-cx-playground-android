package ast

import (
	"runtime/debug"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t2-globals" // thin: only DBG_GOLANG_STACK_TRACE, HeapOffset; core globals, keep
)

// RuntimeErrorRecord is a runtime error of the old loop as data. The loop
// fills it without building text and the printer package prints it after
// the loop has returned, see #413.
type RuntimeErrorRecord struct {
	// RuntimeErrorRecordRaised is set once a runtime error ended the loop.
	RuntimeErrorRecordRaised bool
	// RuntimeErrorRecordCode is the CX exit code, a CX_RUNTIME_* or CX_ASSERT.
	RuntimeErrorRecordCode int
	// RuntimeErrorRecordPanicValue is the value the panic carried. A Go
	// runtime error formats its own message only when it is printed.
	RuntimeErrorRecordPanicValue interface{}
	// RuntimeErrorRecordCall is the call stack position of the failure.
	RuntimeErrorRecordCall types.Pointer
	// RuntimeErrorRecordLine is the expression index in that call.
	RuntimeErrorRecordLine int
	// RuntimeErrorRecordStackPrint is false for a stack overflow at the
	// entry point, which prints no call stack.
	RuntimeErrorRecordStackPrint bool
	// RuntimeErrorRecordAssertLine is set by panicIf, which prints the line
	// and the message in RuntimeErrorRecordAssertMessage before the error.
	RuntimeErrorRecordAssertLine bool
	// RuntimeErrorRecordAssertMessage is the heap offset of the CX string.
	RuntimeErrorRecordAssertMessage types.Pointer
}

// ErrorString ...
func ErrorString(code int) string {
	if str, found := constants.ErrorStrings[code]; found {
		return str
	}
	return constants.ErrorStrings[types.CX_RUNTIME_ERROR]
}

func errorCode(r interface{}) int {
	switch v := r.(type) {
	case int:
		return int(v)
	case RuntimeLibraryDisabled:
		return types.CX_RUNTIME_LIBRARY_DISABLED
	default:
		return types.CX_RUNTIME_ERROR
	}
}

// runtimeErrorRecordSet records the panic value r with the call the program
// stopped in. stackPrint is false for an overflow at the entry point.
func runtimeErrorRecordSet(prgrm *CXProgram, r interface{}, stackPrint bool, defaultError int) {
	call := prgrm.CallStack[prgrm.CallCounter]
	code := errorCode(r)
	if code == types.CX_RUNTIME_ERROR {
		code = defaultError
	}

	record := &prgrm.RuntimeError
	record.RuntimeErrorRecordRaised = true
	record.RuntimeErrorRecordCode = code
	record.RuntimeErrorRecordPanicValue = r
	record.RuntimeErrorRecordCall = prgrm.CallCounter
	record.RuntimeErrorRecordLine = call.Line
	record.RuntimeErrorRecordStackPrint = stackPrint

	// The Go stack of the panic exists only here, so it is printed here.
	if globals.DBG_GOLANG_STACK_TRACE {
		debug.PrintStack()
	}
}

// RuntimeError is deferred by the run loop. It turns a panic into a
// RuntimeErrorRecord on prgrm and lets the loop return.
func RuntimeError(prgrm *CXProgram) {
	if r := recover(); r != nil {
		switch r {
		case constants.STACK_OVERFLOW_ERROR:
			// callPush never pushes a frame that does not fit, so the caller is
			// the current call. Only the entry frame leaves the pointer past the stack.
			stackPrint := prgrm.Stack.Pointer <= constants.STACK_SIZE
			runtimeErrorRecordSet(prgrm, r, stackPrint, types.CX_RUNTIME_STACK_OVERFLOW_ERROR)
		case constants.HEAP_EXHAUSTED_ERROR:
			runtimeErrorRecordSet(prgrm, r, true, types.CX_RUNTIME_HEAP_EXHAUSTED_ERROR)
		default:
			runtimeErrorRecordSet(prgrm, r, true, types.CX_RUNTIME_ERROR)
		}
	}
}
