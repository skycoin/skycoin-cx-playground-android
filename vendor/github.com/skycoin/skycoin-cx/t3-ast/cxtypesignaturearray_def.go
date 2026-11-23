package ast

import "github.com/skycoin/skycoin-cx/t0-types"

type CXTypeSignature_Array struct {
	// if atomic type, Meta is the atomic type
	// if struct array or slice type, Meta
	//  is CXTypeSignature_Struct id
	Meta int

	// length of the array
	// 1-dimensional array [x]
	// 2-dimensional array [x y]
	// 3-dimensional array [x y z]
	Lengths []types.Pointer

	// Index of the element which
	// we want to access.
	Indexes []CXTypeSignatureIndex
}
