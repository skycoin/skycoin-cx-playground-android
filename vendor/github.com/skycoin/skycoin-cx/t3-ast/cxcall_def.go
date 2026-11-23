package ast

import "github.com/skycoin/skycoin-cx/t0-types"

// CXCall ...
type CXCall struct {
	// OBJMODEL: CXFunctionGid, phase 5 #352.
	// Now a pointer at rest, site S0096.
	Operator     *CXFunction   // What CX function will be called when running this CXCall in the runtime
	Line         int           // What line in the CX function is currently being executed
	FramePointer types.Pointer // Where in the stack is this function call's local variables stored
}
