package ast

import "github.com/skycoin/skycoin-cx/t0-types"

// VIOLATION: init chaining. Init 2: the shared ConstCxArg_* templates here and
// in arg_constants_ptr*.go, called from Go package init. RegisterOpCode writes
// their Offset.

// Integer Types, CxArg Constants
// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCXArg_I8 = Param(types.I8)

// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_I16 = Param(types.I16)

// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_I32 = Param(types.I32)

// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_I64 = Param(types.I64)

// Unsigned Integer Types, CxArg Constants
// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_UI8 = Param(types.UI8)

// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_UI16 = Param(types.UI16)

// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_UI32 = Param(types.UI32)

// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_UI64 = Param(types.UI64)

// Floating point, f32, f64, CxArg Constants
// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_F32 = Param(types.F32)

// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_F64 = Param(types.F64)

// ConstCxArg_BOOL Default bool parameter
// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_BOOL = Param(types.BOOL)

// ConstCxArg_STR Default str parameter
// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_STR = Param(types.STR)

// ConstCxArg_UND_TYPE Default und parameter
// VIOLATION: state outside State. Owner: t3-ast; moves to CXArgumentMng in phase 1.
var ConstCxArg_UND_TYPE = Param(types.UNDEFINED)
