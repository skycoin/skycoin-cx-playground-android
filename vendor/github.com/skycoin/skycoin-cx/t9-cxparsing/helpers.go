package cxparsering

import (
	"bytes"

	cxpartialparsing "github.com/skycoin/skycoin-cx/t5-cxpartialparsing"     // thin: only CurrentFile, Parse, Program; no depth gain
	cxparsingcompletor "github.com/skycoin/skycoin-cx/t8-cxparsingcompletor" // thin: only NewLexer, Parse; the pass two parser, keep
)

/*
passone perform cxpartial parsing using partialparsing.y and partialparsing.go
*/
func Passone(source string) int {

	parseErrors := cxpartialparsing.Parse(source)

	return parseErrors
}

/*
passtwo perform cxparsingcompletor parsing using parsingcompletor.y and parsingcompletor.go
*/

func Passtwo(b *bytes.Buffer) int {

	parseErrors := cxparsingcompletor.Parse(cxparsingcompletor.NewLexer(b))

	return parseErrors
}
