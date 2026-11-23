package ast

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

// CXFunction is used to represent a CX function.
type CXFunction struct {
	// Metadata
	Index        int
	Name         stringpool.StringGid // Name of the function
	Package      CXPackageIndex       // The package it's a member of
	AtomicOPCode int

	// Contents
	// OBJMODEL: CXStructGid, phase 5 #337.
	// Now a pointer at rest, site S0127.
	Inputs *CXStruct // Input parameters to the function
	// OBJMODEL: CXStructGid, phase 5 #337.
	// Now a pointer at rest, site S0128.
	Outputs *CXStruct // Output parameters from the function
	// OBJMODEL: []CXExpressionGid or CXExpressionGid, phase 5 #347.
	// Now objects held by value, site S0129.
	Expressions []CXExpression // Expressions, including control flow statements, in the function

	// contains the name of its local variables
	// Used to determine if the variables are to be added to the symbols array
	// Used for local scoping
	LocalVariables []stringpool.StringGid

	//TODO: Better Comment for this
	LineCount int // number of expressions, pre-computed for performance

	//TODO: Better Comment for this
	Size types.Pointer // automatic memory size

	// Debugging
	FileName   SourceFileGid
	FileLine   int
	FileColumn int

	// Used by the GC
	// OBJMODEL: CXArgumentGid, phase 5 #327.
	// Now a pointer at rest, site S0130.
	ListOfPointers []*CXArgument // Root pointers for the GC algorithm

	// EscapeInputs says for each input where the address it holds may go.
	// It is nil until the function is compiled.
	EscapeInputs []CXFunctionEscape

	// ListOfSlots are the roots for the GC inside the locals.
	ListOfSlots []CXFunctionSlot
}
