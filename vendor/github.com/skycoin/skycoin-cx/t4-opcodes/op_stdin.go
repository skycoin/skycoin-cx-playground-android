package opcodes

import (
	"io"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-types"
	// GCREACH: t3-ast owns the heap and the GC roots. This file allocates strs, which
	// adds to the GC pressure, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// OpReadStdin returns the next line of the program's input without its line
// ending. At the end of input it returns "", see LanguageGuide.md.
// VIOLATION STRHOT-098: B, hands a path or text to Go or the OS, #412.
// VIOLATION VMALLOC-018: reads a line into Go memory, #407.
func OpReadStdin(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	var text string
	if prgrm.ProgramStdin != nil {
		var err error
		text, err = prgrm.ProgramStdin.ReadString('\n')
		if err != nil && err != io.EOF {
			panic(types.CX_INTERNAL_ERROR)
		}
	}

	text = strings.Replace(text, "\n", "", -1)
	text = strings.Replace(text, "\r", "", -1)
	outputs[0].Set_str(prgrm, text)
}
