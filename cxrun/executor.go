package cxrun

import (
	"errors"
	"fmt"
	"os"
	"time"

	types "github.com/skycoin/skycoin-cx/t0-types"
	constants "github.com/skycoin/skycoin-cx/t1-constants"
	ir "github.com/skycoin/skycoin-cx/t2-cx-runtime-ir"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
	actions "github.com/skycoin/skycoin-cx/t4-actions"
	lower "github.com/skycoin/skycoin-cx/t4-cx-compiler-lower"
	execute "github.com/skycoin/skycoin-cx/t4-execute"
	printer "github.com/skycoin/skycoin-cx/t5-cx-runtime-printer"
)

// Executor names the loop a program runs on, as the cx --executor flag does.
type Executor string

const (
	// ExecutorOld is the tree walking interpreter, the only one that has the
	// app package.
	ExecutorOld Executor = "old"
	// ExecutorVm is the register vm. It accepts a program only when every
	// function lowers to it.
	ExecutorVm Executor = "vm"
)

// ErrVmRejected wraps the reason the vm does not accept a program.
var ErrVmRejected = errors.New("vm does not accept the program")

// vmOutputSize is the vm output buffer, as in skycoin-cx t11-cx/executor.go.
const vmOutputSize = 64 << 10

// runCompiledOn runs the parsed program on executor and returns the time the
// run took, compile time excluded.
func runCompiledOn(executor Executor) (time.Duration, error) {
	switch executor {
	case ExecutorOld, "":
		started := time.Now()
		err := execute.RunCompiled(actions.AST, 0, nil)
		elapsed := time.Since(started)
		if err != nil {
			return elapsed, err
		}
		if actions.AST.RuntimeError.RuntimeErrorRecordRaised {
			printer.PrinterRuntimeErrorPrint(actions.AST)
			fmt.Println()
			return elapsed, ErrRuntime
		}
		return elapsed, nil
	case ExecutorVm:
		return runOnVm(actions.AST)
	}
	return 0, fmt.Errorf("unknown executor %q", executor)
}

// runOnVm lowers prgrm and runs it the way skycoin-cx t11-cx/executor.go
// does for --executor=vm.
func runOnVm(prgrm *cxast.CXProgram) (time.Duration, error) {
	prgrm.CxIrState.CxIrStateManagerGrowBoundary()
	prgrm.CxIrState.StringPool.StringPoolFreeze()
	image, failures := lower.Program(prgrm)
	switch {
	case len(failures) > 0:
		return 0, fmt.Errorf("%w: %s", ErrVmRejected, vmFailureText(prgrm, failures[0]))
	case image.ImageEntryMain == ir.ImageFuncNone:
		return 0, fmt.Errorf("%w: no main function", ErrVmRejected)
	}
	frames := uint64(image.ImageFuncs[image.ImageEntryMain].FuncFrameSize)
	if image.ImageEntryInit != ir.ImageFuncNone {
		frames += uint64(image.ImageFuncs[image.ImageEntryInit].FuncFrameSize)
	}
	if frames > uint64(image.ImageStackSize) {
		return 0, fmt.Errorf("%w: the frames of *init and main do not fit the stack", ErrVmRejected)
	}
	machine := vm.MachineNew(&image, vmOutputSize)
	started := time.Now()
	framePointer := uint32(0)
	if image.ImageEntryInit != ir.ImageFuncNone {
		if err := runVmEntry(prgrm, machine, image.ImageEntryInit, 0); err != nil {
			return time.Since(started), err
		}
		framePointer = image.ImageFuncs[image.ImageEntryInit].FuncFrameSize
	}
	err := runVmEntry(prgrm, machine, image.ImageEntryMain, framePointer)
	return time.Since(started), err
}

func runVmEntry(prgrm *cxast.CXProgram, machine *vm.Machine, entry, framePointer uint32) error {
	stop, _ := machine.Run(entry, framePointer)
	for {
		if len(machine.MachineOutput) > 0 {
			os.Stdout.Write(machine.MachineOutput)
			machine.MachineOutput = machine.MachineOutput[:0]
		}
		switch {
		case stop == vm.StopReturn:
			return nil
		case stop == vm.StopTestFailed:
			printer.PrinterVmTestFailurePrint(machine)
		case !vm.Resumable(stop):
			printer.PrinterVmErrorPrint(prgrm, machine, stop)
			if vm.StopExitCodes[stop] != types.CX_SUCCESS {
				return ErrRuntime
			}
			return nil
		}
		stop, _ = machine.Resume()
	}
}

func vmFailureText(prgrm *cxast.CXProgram, failure lower.Failure) string {
	text := lower.ReasonNames[failure.FailureReason]
	if failure.FailureReason == lower.ReasonOpcodeOutsideSubset {
		if row, found := constants.OpcodeLifecycleLiveByNumber(int(failure.FailureOpcode)); found {
			text += " " + row.Name
		}
	}
	return text + " in " + prgrm.CXProgramNameText(prgrm.CXFunctions[failure.FailureFunction].Name)
}
