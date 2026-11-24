package cxrun

import (
	"bytes"
	"fmt"

	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	actions "github.com/skycoin/skycoin-cx/t4-actions"
)

// AppOpcodeBase is the first opcode of the app package. Every opcode from
// it up is one of the app calls this package registers.
const AppOpcodeBase = opcodeBase

// InspectSource compiles the .cx files in files, entry first, as RunWith
// does, and calls inspect with the compiled program without running it.
// The program is only valid during the call.
func InspectSource(files map[string][]byte, entry string, inspect func(*cxast.CXProgram) error) error {
	var msgs bytes.Buffer
	err := compileWith(files, entry, Options{}, nil, &msgs, func() error {
		return inspect(actions.AST)
	})
	if err == ErrCompile {
		return fmt.Errorf("%w\n%s", err, msgs.Bytes())
	}
	return err
}

// InspectIR loads a program that Serialize made, as RunIRWith does, and
// calls inspect with it without running it. The program is only valid
// during the call.
func InspectIR(program []byte, inspect func(*cxast.CXProgram) error) (err error) {
	runMu.Lock()
	defer runMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cx panic: %v", r)
		}
	}()
	resetCompiler()
	prgrm, err := cxast.Deserialize(program, nil)
	if err != nil {
		return err
	}
	return inspect(prgrm)
}
