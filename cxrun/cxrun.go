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
	"time"

	packages "github.com/skycoin/skycoin-cx/t0-packages"
	util "github.com/skycoin/skycoin-cx/t0-util"
	constants "github.com/skycoin/skycoin-cx/t1-constants"
	globals "github.com/skycoin/skycoin-cx/t2-globals"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	actions "github.com/skycoin/skycoin-cx/t4-actions"
	opcodes "github.com/skycoin/skycoin-cx/t4-opcodes"
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
	// CellSize is the size in pixels of one character of the monospaced
	// app font, the cell of a terminal grid.
	CellSize() (w, h float32)
}

var (
	runMu sync.Mutex
	host  Host
	cmds  []Cmd
)

func init() {
	packages.DefaultPackages = append(packages.DefaultPackages, "app")
	actions.InitConstants()
	cxos.InitConstants(actions.AddConstI32)
	resetCompiler()
}

// resetCompiler starts from a new program with every native registered again,
// the way skycoin-cx's own tests run many programs in one process.
func resetCompiler() {
	actions.AST = cxinit.MakeProgram()
	cxinit.RegisterPackages(actions.AST)
	opcodes.RegisterOpcodes(actions.AST)
	register(actions.AST)
	cxast.OpcodeHandlers[opcodes.OpcodeNumber("os.Exit")] = opExit
	globals.FoundCompileErrors = false
	actions.LineNo = 0
	actions.TemporaryVariableCounter = 0
}

// ErrCompile is returned when the sources do not compile. The compiler's
// messages are in the output.
var ErrCompile = errors.New("compilation failed")

// ErrRuntime is returned when the program stops on a runtime error, which is
// in the output.
var ErrRuntime = errors.New("runtime error")

// Run compiles the .cx files in files, entry first, and runs main on the old
// executor. Program output, including compiler errors, is written to out.
// Only one program runs at a time.
func Run(files map[string][]byte, entry string, h Host, out io.Writer) error {
	_, err := RunOn(files, entry, ExecutorOld, h, out)
	return err
}

// Options are the optional settings of a run.
type Options struct {
	// Args is the program's os.Args, as the ++ arguments of cx.
	Args []string
	// Dir is the sandbox root that the program's file operations resolve
	// against. Empty leaves the program without one.
	Dir string
	// Stderr receives the program's standard error. Nil sends it to the
	// output with standard output.
	Stderr io.Writer
	// Libraries names the system libraries the program may call, from
	// LibrariesAll. A call into another one stops the program with
	// CX_RUNTIME_LIBRARY_DISABLED.
	Libraries []string
}

// RunWith is Run with options.
func RunWith(files map[string][]byte, entry string, opts Options, h Host, out io.Writer) error {
	_, err := runOn(files, entry, ExecutorOld, opts, h, out)
	return err
}

// RunOn is Run on executor. It returns how long main ran, compile time
// excluded. h may be nil for a program that does not use the app package.
func RunOn(files map[string][]byte, entry string, executor Executor, h Host, out io.Writer) (elapsed time.Duration, err error) {
	return runOn(files, entry, executor, Options{}, h, out)
}

func runOn(files map[string][]byte, entry string, executor Executor, opts Options, h Host, out io.Writer) (elapsed time.Duration, err error) {
	err = compileWith(files, entry, opts, h, out, func() error {
		elapsed, err = runCompiledOn(executor, opts.Args)
		return err
	})
	return elapsed, err
}

// compileWith compiles the .cx files in files, entry first, into actions.AST
// with output going to out, and then calls then.
func compileWith(files map[string][]byte, entry string, opts Options, h Host, out io.Writer, then func() error) (err error) {
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

	restore, err := captureOutput(out, opts.Stderr)
	if err != nil {
		return err
	}
	defer restore()

	host, cmds = h, cmds[:0]
	defer func() { host = nil; netCloseAll() }()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cx panic: %v", r)
		}
	}()

	resetCompiler()
	if opts.Dir != "" {
		root, fileError := util.ProgramSandboxRootOpen(opts.Dir)
		if fileError != nil {
			return errors.New(fileError.FileErrorMessage)
		}
		defer root.Close()
		actions.AST.ProgramSandboxRoot = root
	}
	if err := cxparsing.ParseSourceStrings(srcs, names); err != nil {
		return ErrCompile
	}
	if _, err := actions.AST.GetFunction(actions.AST.CXProgramNameIntern(constants.MAIN_FUNC), actions.AST.CxIrState.CxIrStateNameMain); err != nil {
		return errors.New("no main function in package main")
	}
	if err := cxparsing.AddInitFunction(actions.AST); err != nil {
		return err
	}
	if actions.AST.ProgramLibrariesEnabled, err = librariesEnabled(opts.Libraries); err != nil {
		return err
	}
	return then()
}

// captureOutput sends os.Stdout to out, and os.Stderr to stderr or to out
// when stderr is nil, until restore is called. CX prints with fmt.Println
// and compile errors and warnings go to os.Stderr.
func captureOutput(out, stderr io.Writer) (restore func(), err error) {
	oldOut, oldErr := os.Stdout, os.Stderr
	var closers []func()
	pipe := func(dst io.Writer) (*os.File, error) {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		done := make(chan struct{})
		go func() {
			io.Copy(dst, r)
			close(done)
		}()
		closers = append(closers, func() { w.Close(); <-done; r.Close() })
		return w, nil
	}
	w, err := pipe(out)
	if err != nil {
		return nil, err
	}
	wErr := w
	if stderr != nil {
		if wErr, err = pipe(stderr); err != nil {
			closers[0]()
			return nil, err
		}
	}
	os.Stdout, os.Stderr = w, wErr
	return func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		for _, c := range closers {
			c()
		}
	}, nil
}
