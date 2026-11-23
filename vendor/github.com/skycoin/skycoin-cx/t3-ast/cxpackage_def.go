package ast

import stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"

// CXPackage is used to represent a CX package.
type CXPackage struct {
	// Metadata
	Name  stringpool.StringGid // Name of the package
	Index int                  // Index of package inside the CXPackage array

	// Contents
	Imports   map[stringpool.StringGid]CXPackageIndex  // imported packages
	Functions map[stringpool.StringGid]CXFunctionIndex // declared functions in this package
	Structs   map[stringpool.StringGid]CXStructIndex   // declared structs in this package
	// Methods finds a method by its receiver struct and method name. A method
	// is also in Functions under its "Struct.method" name.
	Methods map[CXPackageMethodKey]CXFunctionIndex
	// OBJMODEL: CXStructGid, phase 5 #337.
	// Now a pointer at rest, site S0157.
	Globals *CXStruct // declared global variables in this package

	// Used by the REPL and cxgo
	CurrentFunction CXFunctionIndex
}
