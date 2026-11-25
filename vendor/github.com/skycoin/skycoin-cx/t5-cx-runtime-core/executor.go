package core

import (
	"fmt"
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t2-cx-runtime-ir"
	// GCREACH: t3-ast owns the heap and the GC roots. ExecutorRun runs vm, which has no
	// heap, or the old loop, which collects, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
	lower "github.com/skycoin/skycoin-cx/t4-cx-compiler-lower"
	"github.com/skycoin/skycoin-cx/t4-cx-runtime-printer"
	"github.com/skycoin/skycoin-cx/t4-execute"
	"github.com/skycoin/skycoin-cx/t4-opcodes"
)

// The --executor values. auto runs a program on vm when every function
// lowers and on the old loop otherwise, step 5 of #408.
const (
	ExecutorAuto = "auto"
	ExecutorOld  = "old"
	ExecutorVm   = "vm"
)

// ExecutorOutputSize is the output buffer of a vm run.
const ExecutorOutputSize = 64 << 10

// ExecutorLower lowers prgrm. It returns "" when the program can run on vm:
// every function lowered, there is a main function, and the frames of *init
// and main fit the stack, which the old loop checks only for main. Otherwise
// it returns why not.
func ExecutorLower(prgrm *ast.CXProgram) (image ir.Image, failures []lower.Failure, outside string) {
	image, failures = lower.Program(prgrm)
	switch {
	case len(failures) > 0:
		return image, failures, executorFailureDescribe(prgrm, failures[0])
	case image.ImageEntryMain == ir.ImageFuncNone:
		return image, failures, "no main function"
	}
	entries := uint64(image.ImageFuncs[image.ImageEntryMain].FuncFrameSize)
	if image.ImageEntryInit != ir.ImageFuncNone {
		entries += uint64(image.ImageFuncs[image.ImageEntryInit].FuncFrameSize)
	}
	if entries > uint64(image.ImageStackSize) {
		return image, failures, "the frames of *init and main do not fit the stack"
	}
	return image, failures, ""
}

// executorFailureDescribe names the reason and function of a failure.
func executorFailureDescribe(prgrm *ast.CXProgram, failure lower.Failure) string {
	description := lower.ReasonNames[failure.FailureReason]
	if failure.FailureReason == lower.ReasonOpcodeOutsideSubset {
		if row, found := constants.OpcodeLifecycleLiveByNumber(int(failure.FailureOpcode)); found {
			description += " " + row.Name
		}
	}
	return description + " in " + prgrm.CXProgramNameText(prgrm.CXFunctions[failure.FailureFunction].Name)
}

// ExecutorVmRun runs image on vm as execute.RunCompiled and runProgram run
// prgrm, writing to os.Stdout. It returns the exit code: CX_SUCCESS,
// CX_ASSERT after a failed test, or the code of a runtime error, which it
// prints like printer.PrinterRuntimeErrorPrint.
func ExecutorVmRun(prgrm *ast.CXProgram, image *ir.Image) int32 {
	return ExecutorVmRunMachine(prgrm, vm.MachineNew(image, ExecutorOutputSize))
}

// ExecutorVmRunMachine runs the image of machine from a reset machine, see
// ExecutorVmRun.
func ExecutorVmRunMachine(prgrm *ast.CXProgram, machine *vm.Machine) int32 {
	image := machine.MachineImage
	machine.Reset()
	framePointer := uint32(0)
	if image.ImageEntryInit != ir.ImageFuncNone {
		if code := executorVmRunEntry(prgrm, machine, image.ImageEntryInit, 0); code != types.CX_SUCCESS {
			return code
		}
		framePointer = image.ImageFuncs[image.ImageEntryInit].FuncFrameSize
	}
	if code := executorVmRunEntry(prgrm, machine, image.ImageEntryMain, framePointer); code != types.CX_SUCCESS {
		return code
	}
	if machine.MachineTestFailures > 0 {
		return types.CX_ASSERT
	}
	return types.CX_SUCCESS
}

// executorVmRunEntry runs one entry function, serving the resumable stops.
func executorVmRunEntry(prgrm *ast.CXProgram, machine *vm.Machine, entry uint32, framePointer uint32) int32 {
	stop, _ := machine.Run(entry, framePointer)
	for {
		executorOutputFlush(machine)
		switch {
		case stop == vm.StopReturn:
			return types.CX_SUCCESS
		case stop == vm.StopTestFailed:
			printer.PrinterVmTestFailurePrint(machine)
		case !vm.Resumable(stop):
			printer.PrinterVmErrorPrint(prgrm, machine, stop)
			return vm.StopExitCodes[stop]
		}
		stop, _ = machine.Resume()
	}
}

// executorOutputFlush writes and empties the output of machine.
func executorOutputFlush(machine *vm.Machine) {
	if len(machine.MachineOutput) == 0 {
		return
	}
	// A failed write to stdout is ignored, as fmt.Println in the old loop
	// ignores it.
	os.Stdout.Write(machine.MachineOutput)
	machine.MachineOutput = machine.MachineOutput[:0]
}

// ExecutorMemoryLimitsClamp keeps the heap and stack sizes below
// constants.ESCAPE_OFFSET_FLAG, after the flags of cx have set them.
func ExecutorMemoryLimitsClamp() {
	constants.MAX_HEAP_SIZE = min(constants.MAX_HEAP_SIZE, constants.ESCAPE_OFFSET_FLAG/2)
	constants.INIT_HEAP_SIZE = min(constants.INIT_HEAP_SIZE, constants.MAX_HEAP_SIZE)
	constants.STACK_SIZE = min(constants.STACK_SIZE, constants.ESCAPE_OFFSET_FLAG/4)
}

// ExecutorRun runs the compiled or deserialized prgrm with executor, one of
// ExecutorAuto, ExecutorOld and ExecutorVm, and cxArgs as os.Args. It
// returns the exit code, after printing any runtime error.
func ExecutorRun(prgrm *ast.CXProgram, executor string, cxArgs []string) int32 {
	CoreLibrariesEnabledResolve(prgrm)
	for library, start := range ast.LibraryRunStarts {
		if start == nil || !prgrm.ProgramLibrariesEnabled[library] {
			continue
		}
		if err := start(); err != nil {
			fmt.Fprintf(os.Stderr, "library %s: %s\n", constants.LibraryNameOf(library), err)
			return types.CX_INTERNAL_ERROR
		}
	}
	prgrm.CxIrState.CxIrStateManagerGrowBoundary()
	prgrm.CxIrState.StringPool.StringPoolFreeze()

	if executor != ExecutorOld {
		image, _, outside := ExecutorLower(prgrm)
		switch {
		case outside == "":
			return ExecutorVmRun(prgrm, &image)
		case executor == ExecutorVm:
			fmt.Fprintf(os.Stderr, "cannot run on vm: %s\n", outside)
			return types.CX_RUNTIME_NOT_IMPLEMENTED
		}
	}

	execute.RunPrepare(prgrm)
	if err := execute.RunCompiled(prgrm, 0, cxArgs); err != nil {
		panic(err)
	}
	if prgrm.RuntimeError.RuntimeErrorRecordRaised {
		return int32(printer.PrinterRuntimeErrorPrint(prgrm))
	}
	if opcodes.AssertFailed() {
		return types.CX_ASSERT
	}
	return types.CX_SUCCESS
}
