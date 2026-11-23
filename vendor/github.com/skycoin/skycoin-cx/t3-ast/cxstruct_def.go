package ast

import stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"

// CXStruct is used to represent a CX struct.
type CXStruct struct {
	// Metadata
	Index   int
	Name    stringpool.StringGid // Name of the struct
	Package CXPackageIndex       // The package this struct belongs to

	// Contents
	Fields []CXTypeSignatureIndex // The fields of the struct

	// Debugging, the position of the name of a declared type, line 0 before
	// pass one declares it
	FileName   SourceFileGid
	FileLine   int
	FileColumn int
}
