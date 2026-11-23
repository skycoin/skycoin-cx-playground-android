//go:build !noregexp

package regexp

import (
	"regexp"

	"github.com/skycoin/skycoin-cx/t0-types" // thin: only Read_str, STR, Write_str; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// VIOLATION: state outside State. Owner: t5-regexp; moves to RegexpMng in phase 1.
var regexps map[string]*regexp.Regexp = make(map[string]*regexp.Regexp, 0)

// expOffset returns the offset of the `exp` field of the CX `regexp.Regexp`
// held by value. The field holds the expression, which keys `regexps`. exp is
// the only field RegisterPackage gives Regexp, so it starts the struct.
func expOffset(value ast.CXValue) types.Pointer {
	return value.Offset
}

// regexpCompile is a helper function for `opRegexpMustCompile` and
// `opRegexpCompile`. `regexpCompile` compiles a `regexp.Regexp` structure
// and adds it to global `regexps`. It also writes CX structure `regexp.Regexp`.
func regexpCompile(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) error {
	exp := inputs[0].Get_str(prgrm)

	// The CX Regexp only holds the expression. The same expression in two
	// parts of a program shares one compiled instance.
	types.Write_str(prgrm, prgrm.Memory, expOffset(outputs[0]), exp)

	r, err := regexp.Compile(exp)
	if err != nil {
		return err
	}
	regexps[exp] = r
	return nil
}

// opRegexpMustCompile is a wrapper for golang's `regexp`'s `MustCompile`.
func opRegexpMustCompile(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	err := regexpCompile(prgrm, inputs, outputs)

	if err != nil {
		println(err.Error())
		panic(types.CX_RUNTIME_ERROR)
	}

}

// opRegexpCompile is a wrapper for golang's `regexp`'s `MustCompile`.
func opRegexpCompile(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	// We're only interested in `out1`, which represents the
	// returned error.
	err := regexpCompile(prgrm, inputs, outputs)

	// Writing error message to `out1`.
	var errStr string
	if err != nil {
		errStr = err.Error()
	}
	outputs[1].Set_str(prgrm, errStr)
}

// opRegexpFind is a wrapper for golang's `regexp`'s `Find`.
func opRegexpFind(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	exp := types.Read_str(prgrm.Memory, expOffset(inputs[0]))
	r, ok := regexps[exp]
	if !ok {
		println("regexp: Find on a Regexp that did not compile: " + exp)
		panic(types.CX_RUNTIME_ERROR)
	}

	outputs[0].Set_str(prgrm, string(r.Find([]byte(inputs[1].Get_str(prgrm)))))
}
