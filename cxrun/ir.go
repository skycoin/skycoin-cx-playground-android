package cxrun

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	util "github.com/skycoin/skycoin-cx/t0-util"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	actions "github.com/skycoin/skycoin-cx/t4-actions"
	printer "github.com/skycoin/skycoin-cx/t5-cx-runtime-printer"
)

// ExecutorAuto runs a program on the vm when every function lowers to it and
// on the old executor otherwise, as cx --executor=auto and the cx build core
// (skycoin-cx t6-cx-runtime-core ExecutorRun) do.
const ExecutorAuto Executor = "auto"

// Serialize compiles the .cx files in files, entry first, as RunWith does and
// returns the program serialized with its memory, uncompressed: the bytes
// that cx build appends to its core. Compiler messages go to the error.
func Serialize(files map[string][]byte, entry string) ([]byte, error) {
	var msgs bytes.Buffer
	var program []byte
	err := compileWith(files, entry, Options{}, nil, &msgs, func() error {
		var err error
		program, err = cxast.SerializeCXProgram(actions.AST, true, nil)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("%w\n%s", err, msgs.Bytes())
	}
	return program, nil
}

// RunIRWith loads a program that Serialize made and runs it on executor, with
// no parser involved. The compiler state is reset first, which registers the
// opcodes, the app package among them, in the order Serialize saw them.
func RunIRWith(program []byte, executor Executor, opts Options, h Host, out io.Writer) (elapsed time.Duration, err error) {
	runMu.Lock()
	defer runMu.Unlock()
	restore, err := captureOutput(out, opts.Stderr)
	if err != nil {
		return 0, err
	}
	defer restore()
	host, cmds = h, cmds[:0]
	defer func() { host = nil; netRunStop() }()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cx panic: %v", r)
		}
	}()

	resetCompiler()
	prgrm, err := cxast.Deserialize(program, nil)
	if err != nil {
		return 0, err
	}
	printer.PrinterCompileErrorsRegister(prgrm)
	actions.AST = prgrm
	// The serialized program carries the libraries enabled when it was
	// built. The run decides instead.
	if prgrm.ProgramLibrariesEnabled, err = librariesEnabled(opts.Libraries); err != nil {
		return 0, err
	}
	if opts.Dir != "" {
		root, fileError := util.ProgramSandboxRootOpen(opts.Dir)
		if fileError != nil {
			return 0, errors.New(fileError.FileErrorMessage)
		}
		defer root.Close()
		prgrm.ProgramSandboxRoot = root
	}
	if err := netRunStart(opts); err != nil {
		return 0, err
	}
	return runCompiledOn(executor, opts.Args)
}
