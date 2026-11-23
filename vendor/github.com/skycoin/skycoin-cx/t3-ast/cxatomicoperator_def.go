package ast

import stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"

type CXAtomicOperator struct {
	// OBJMODEL: CXStructGid, phase 5 #337.
	// Now a pointer at rest, site S0087.
	Inputs *CXStruct
	// OBJMODEL: CXStructGid, phase 5 #337.
	// Now a pointer at rest, site S0088.
	Outputs  *CXStruct
	Operator CXFunctionIndex

	Function CXFunctionIndex
	Package  CXPackageIndex
	Label    stringpool.StringGid

	// used for jmp statements
	ThenLines int
	ElseLines int
}
