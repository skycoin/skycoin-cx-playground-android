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
	parsingcompletor "github.com/skycoin/skycoin-cx/t8-cxparsingcompletor"
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
	saved map[*cxast.CXArgument]cxast.CXArgument
)

func init() {
	types.Allocator = cxast.AllocateSeq
	packages.DefaultPackages = append(packages.DefaultPackages, "app")
	parsingcompletor.InitCXCore()
	register(actions.AST)
	saved = map[*cxast.CXArgument]cxast.CXArgument{}
	walkNatives(func(a *cxast.CXArgument) { saved[a] = copyArg(*a) })
}

// The parser writes into the argument structs shared by every native
// function, so a second program sees types left over from the first.
// Restoring them and starting from an empty program lets one process run
// any number of programs.
func copyArg(a cxast.CXArgument) cxast.CXArgument {
	a.Lengths = append(a.Lengths[:0:0], a.Lengths...)
	a.DereferenceOperations = append(a.DereferenceOperations[:0:0], a.DereferenceOperations...)
	a.DeclarationSpecifiers = append(a.DeclarationSpecifiers[:0:0], a.DeclarationSpecifiers...)
	return a
}

func walkNatives(f func(*cxast.CXArgument)) {
	visit := func(n *cxast.CXNativeFunction) {
		if n == nil {
			return
		}
		for _, a := range n.Inputs {
			f(a)
		}
		for _, a := range n.Outputs {
			f(a)
		}
	}
	for _, n := range cxast.Natives {
		visit(n)
	}
	for _, n := range cxast.Operators {
		visit(n)
	}
}

func resetCompiler() {
	walkNatives(func(a *cxast.CXArgument) { *a = copyArg(saved[a]) })
	heap := cxast.MinHeapSize()
	actions.AST = &cxast.CXProgram{
		Packages:       make(map[string]cxast.CXPackageIndex),
		CXFunctions:    make([]cxast.CXFunction, 0),
		CurrentPackage: -1,
		CallStack:      make([]cxast.CXCall, constants.CALLSTACK_SIZE),
		Memory:         make([]byte, constants.STACK_SIZE+heap),
		Stack:          cxast.StackSegmentStruct{Size: constants.STACK_SIZE},
		Data:           cxast.DataSegmentStruct{StartsAt: constants.STACK_SIZE},
		Heap:           cxast.HeapSegmentStruct{Size: heap, Pointer: constants.NULL_HEAP_ADDRESS_OFFSET},
		CXArgs:         make([]cxast.CXArgument, 0),
	}
	globals.FoundCompileErrors = false
	actions.LineNo = 0
}

// ErrCompile is returned when the sources do not compile. The compiler's
// messages are in the output.
var ErrCompile = errors.New("compilation failed")

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

	restore, err := captureStdout(out)
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
	if _, err := actions.AST.GetFunction(constants.MAIN_FUNC, constants.MAIN_PKG); err != nil {
		return errors.New("no main function in package main")
	}
	if err := cxparsing.AddInitFunction(actions.AST); err != nil {
		return err
	}
	return execute.RunCompiled(actions.AST, 0, nil)
}

// captureStdout sends os.Stdout to out until restore is called. CX prints
// with fmt.Println.
func captureStdout(out io.Writer) (restore func(), err error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		io.Copy(out, r)
		close(done)
	}()
	return func() {
		os.Stdout = old
		w.Close()
		<-done
		r.Close()
	}, nil
}
