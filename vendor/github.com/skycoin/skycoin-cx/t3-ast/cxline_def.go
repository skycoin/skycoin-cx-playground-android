package ast

import stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"

// CXLine is the source position of the expressions after it. A LineColumn of
// 0 is unknown.
type CXLine struct {
	FileName   SourceFileGid
	LineNumber int
	LineColumn int
	LineStr    stringpool.StringGid
}
