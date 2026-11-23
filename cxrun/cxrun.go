// Package cxrun compiles and runs CX programs one after another in the same
// process and gives them the "app" package: a display list, an event queue
// and the soft keyboard, implemented by a Host.
package cxrun

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	packages "github.com/skycoin/skycoin-cx/t0-packages"
	types "github.com/skycoin/skycoin-cx/t0-types"
	constants "github.com/skycoin/skycoin-cx/t1-constants"
	globals "github.com/skycoin/skycoin-cx/t2-globals"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	actions "github.com/skycoin/skycoin-cx/t4-actions"
	execute "github.com/skycoin/skycoin-cx/t4-execute"
	opcodes "github.com/skycoin/skycoin-cx/t4-opcodes"
	printer "github.com/skycoin/skycoin-cx/t5-cx-runtime-printer"
	cxos "github.com/skycoin/skycoin-cx/t5-cxos"
	cxinit "github.com/skycoin/skycoin-cx/t7-init"
	cxparsing "github.com/skycoin/skycoin-cx/t9-cxparsing"
)

// Event kinds seen by CX through app.PollEvent.
const (
	EvNone = iota
	EvTouchDown
	EvTouchMove
	EvTouchUp
	EvKey
	EvResize
)

// Codes of EvKey events for keys that have no rune.
const (
	KeyBackspace = 8
	KeyEnter     = 10
)

// Event is one input event, in pixels relative to the program's canvas.
type Event struct {
	Kind int
	X, Y float32
	Code int32
}

// Cmd kinds in a display list.
const (
	CmdClear = iota
	CmdRect
	CmdCircle
	CmdLine
	CmdTri
	CmdText
)

// Cmd is one drawing command. Coordinates are canvas pixels.
type Cmd struct {
	Kind       int
	P          [6]float32
	Width      float32
	Text       string
	R, G, B, A float32
}

// Host is what a running program draws to and reads input from.
type Host interface {
	// Size is the canvas size in pixels.
	Size() (w, h int)
	// Frame shows the display list and returns once it is on screen.
	Frame(cmds []Cmd)
	// PollEvent returns the next queued event.
	PollEvent() (Event, bool)
	// Keyboard shows or hides the soft keyboard.
	Keyboard(show bool)
	// ShouldClose reports that the user asked to leave the program.
	ShouldClose() bool
}

var (
	runMu sync.Mutex
	host  Host
	cmds  []Cmd
)

func init() {
	types.Allocator = cxast.AllocateSeq
	packages.DefaultPackages = append(packages.DefaultPackages, "app")
	actions.InitConstants()
	cxos.InitConstants()
	resetCompiler()
}

// resetCompiler starts from a new program with every native registered again,
// the way skycoin-cx's own tests run many programs in one process.
func resetCompiler() {
	actions.AST = cxinit.MakeProgram()
	cxinit.RegisterPackages(actions.AST)
	opcodes.RegisterOpcodes(actions.AST)
	register(actions.AST)
	globals.FoundCompileErrors = false
	actions.LineNo = 0
}

// ErrCompile is returned when the sources do not compile. The compiler's
// messages are in the output.
var ErrCompile = errors.New("compilation failed")

// ErrRuntime is returned when the program stops on a runtime error, which is
// in the output.
var ErrRuntime = errors.New("runtime error")

// Run compiles the .cx files in files, entry first, and runs main. Program
// output, including compiler errors, is written to out. Only one program
// runs at a time.
func Run(files map[string][]byte, entry string, h Host, out io.Writer) (err error) {
	runMu.Lock()
	defer runMu.Unlock()

	names := make([]string, 0, len(files))
	for n := range files {
		if strings.HasSuffix(n, ".cx") && n != entry {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	if _, ok := files[entry]; ok {
		names = append([]string{entry}, names...)
	}
	if len(names) == 0 {
		return errors.New("no .cx files")
	}
	srcs := make([]string, len(names))
	for i, n := range names {
		srcs[i] = string(files[n])
	}

	restore, err := captureOutput(out)
	if err != nil {
		return err
	}
	defer restore()

	host, cmds = h, cmds[:0]
	defer func() { host = nil }()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cx panic: %v", r)
		}
	}()

	resetCompiler()
	if err := cxparsing.ParseSourceStrings(srcs, names); err != nil {
		return ErrCompile
	}
	if _, err := actions.AST.GetFunction(actions.AST.CXProgramNameIntern(constants.MAIN_FUNC), actions.AST.CxIrState.CxIrStateNameMain); err != nil {
		return errors.New("no main function in package main")
	}
	if err := cxparsing.AddInitFunction(actions.AST); err != nil {
		return err
	}
	if err := execute.RunCompiled(actions.AST, 0, nil); err != nil {
		return err
	}
	if actions.AST.RuntimeError.RuntimeErrorRecordRaised {
		printer.PrinterRuntimeErrorPrint(actions.AST)
		fmt.Println()
		return ErrRuntime
	}
	return nil
}

// captureOutput sends os.Stdout and os.Stderr to out until restore is called.
// CX prints with fmt.Println and compile errors go to os.Stderr.
func captureOutput(out io.Writer) (restore func(), err error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	done := make(chan struct{})
	go func() {
		io.Copy(out, r)
		close(done)
	}()
	return func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		w.Close()
		<-done
		r.Close()
	}, nil
}
