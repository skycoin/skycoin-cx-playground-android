package ast

// CXNativeFunction is used to represent built-in operations.
type CXNativeFunction struct {
	// Metadata
	AtomicOPCode int

	// Contents
	// OBJMODEL: CXArgumentGid, phase 5 #327.
	// Now a pointer at rest, site S0131.
	Inputs []*CXArgument // Input parameters to the function
	// OBJMODEL: CXArgumentGid, phase 5 #327.
	// Now a pointer at rest, site S0132.
	Outputs []*CXArgument // Output parameters from the function
}
