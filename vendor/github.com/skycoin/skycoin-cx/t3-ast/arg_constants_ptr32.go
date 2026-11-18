//go:build !ptr64

package ast

import "github.com/skycoin/skycoin-cx/t0-types"

// Pointer type, CXArg Constants
var ConstCxArg_PTR = Param(types.I32) // TODO:PTR use right type when we have ptr alias in cx
