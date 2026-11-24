package ast

// CXNativeFunction is used to represent built-in operations.
type CXNativeFunction struct {
	// Metadata
	AtomicOPCode int

	// LibraryNumber and LibraryFunctionNumber name the system library function
	// an AtomicOPCode of constants.OP_LIBRARY_CALL calls, and are 0 otherwise.
	LibraryNumber         int
	LibraryFunctionNumber int

	// Contents
	// OBJMODEL: CXArgumentGid, phase 5 #327.
	// Now a pointer at rest, site S0131.
	Inputs []*CXArgument // Input parameters to the function
	// OBJMODEL: CXArgumentGid, phase 5 #327.
	// Now a pointer at rest, site S0132.
	Outputs []*CXArgument // Output parameters from the function
}
