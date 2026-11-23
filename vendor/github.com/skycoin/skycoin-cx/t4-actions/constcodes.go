package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
)

// For the cxgo. These shouldn't be used in the runtime for performance reasons
var (
	// VIOLATION: state outside State. Owner: t4-actions; moves to CXConstantMng in phase 1.
	ConstCodes = map[string]int{} //Todo: Used only once in actions/postfix.go
	// VIOLATION: state outside State. Owner: t4-actions; moves to CXConstantMng in phase 1.
	Constants = map[int]CXConstant{} //Todo: Only used once in actions/postfix.go
)

// constPointerSize is the code of cx.POINTER_SIZE, after the exit codes.
const constPointerSize = types.CX_RUNTIME_NOT_IMPLEMENTED + 1

// nolint typecheck
// InitConstants registers the cx.* constants in ConstCodes and Constants.
func InitConstants() {
	// cx
	AddConstI32(types.CX_SUCCESS, "cx.SUCCESS", types.CX_SUCCESS)
	AddConstI32(types.CX_COMPILATION_ERROR, "cx.COMPILATION_ERROR", types.CX_COMPILATION_ERROR)
	AddConstI32(types.CX_PANIC, "cx.PANIC", types.CX_PANIC)
	AddConstI32(types.CX_INTERNAL_ERROR, "cx.INTERNAL_ERROR", types.CX_INTERNAL_ERROR)
	AddConstI32(types.CX_ASSERT, "cx.ASSERT", types.CX_ASSERT)
	AddConstI32(types.CX_RUNTIME_ERROR, "cx.RUNTIME_ERROR", types.CX_RUNTIME_ERROR)
	AddConstI32(types.CX_RUNTIME_STACK_OVERFLOW_ERROR, "cx.RUNTIME_STACK_OVERFLOW_ERROR", types.CX_RUNTIME_STACK_OVERFLOW_ERROR)
	AddConstI32(types.CX_RUNTIME_HEAP_EXHAUSTED_ERROR, "cx.RUNTIME_HEAP_EXHAUSTED_ERROR", types.CX_RUNTIME_HEAP_EXHAUSTED_ERROR)
	AddConstI32(types.CX_RUNTIME_INVALID_ARGUMENT, "cx.RUNTIME_INVALID_ARGUMENT", types.CX_RUNTIME_INVALID_ARGUMENT)
	AddConstI32(types.CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE, "cx.RUNTIME_SLICE_INDEX_OUT_OF_RANGE", types.CX_RUNTIME_SLICE_INDEX_OUT_OF_RANGE)
	AddConstI32(types.CX_RUNTIME_NOT_IMPLEMENTED, "cx.RUNTIME_NOT_IMPLEMENTED", types.CX_RUNTIME_NOT_IMPLEMENTED)
	AddConstI32(constPointerSize, "cx.POINTER_SIZE", types.Cast_ptr_to_i32(types.POINTER_SIZE))
}

// AddConstCode ...
func AddConstCode(code int, name string, typeCode types.Code, value []byte) {
	ConstCodes[name] = code
	Constants[code] = CXConstant{Type: typeCode, Value: value}
}

// AddConstI32 ...
func AddConstI32(code int, name string, value int32) {
	var memory [4]byte
	types.Write_i32(memory[:], 0, value)
	AddConstCode(code, name, types.I32, memory[:])
}

// CXConstant ...
type CXConstant struct {
	// native constants. only used for pre-packaged constants (e.g. math package's PI)
	// these fields are used to feed WritePrimary
	Value []byte
	Type  types.Code
}
