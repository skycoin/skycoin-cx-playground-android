package printer

import (
	"fmt"
	"runtime/debug"
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t2-cx-runtime-ir"
	"github.com/skycoin/skycoin-cx/t2-globals"
	"github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
)

// printerVmStopMessages is what the old loop prints after the error string
// for each vm error, the value its panic carried.
var printerVmStopMessages = [vm.StopCount]string{
	vm.StopDivideByZero:   "runtime error: integer divide by zero",
	vm.StopStackOverflow:  constants.STACK_OVERFLOW_ERROR,
	vm.StopOpcodeInvalid:  "invalid image opcode",
	vm.StopIndexNegative:  strconv.Itoa(types.CX_RUNTIME_INVALID_CAST),
	vm.StopAddressInvalid: "address outside the stack and data segment",
}

// printerVmStopLine returns the file and line of the instruction machine
// stopped at, "" and 0 when it has none, as GetPreviousCXLine gives them.
func printerVmStopLine(machine *vm.Machine) (string, int) {
	image := machine.MachineImage
	line, found := ir.LineAt(image, machine.MachineFunction, machine.MachinePc)
	if !found {
		return "", 0
	}
	return image.ImageFileNames[line.LineFile], int(line.LineNumber)
}

// PrinterVmTestFailurePrint prints a failed test as assert in t4-opcodes
// does.
func PrinterVmTestFailurePrint(machine *vm.Machine) {
	fileName, lineNumber := printerVmStopLine(machine)
	message := machine.TestMessage()
	if len(message) > 0 {
		fmt.Printf("%s: %d: result was not equal to the expected value; %s\n", fileName, lineNumber, message)
	} else {
		fmt.Printf("%s: %d: result was not equal to the expected value\n", fileName, lineNumber)
	}
}

// PrinterVmErrorPrint prints the runtime error vm stopped with as the old
// loop prints it, the call stack included. PrinterStackPrint reads the old
// program state, so the vm memory and frames are copied into prgrm first.
func PrinterVmErrorPrint(prgrm *ast.CXProgram, machine *vm.Machine, stop vm.Stop) {
	image := machine.MachineImage
	fileName, lineNumber := printerVmStopLine(machine)
	code := int(vm.StopExitCodes[stop])
	message := printerVmStopMessages[stop]
	if stop == vm.StopLibraryDisabled {
		library := image.ImageFuncs[machine.MachineFunction].FuncCode[machine.MachinePc].InstrM
		message = ast.RuntimeLibraryDisabledMessage(int(library))
	}
	fmt.Printf("%s, %s, %v", PrinterErrorHeader(fileName, lineNumber), ast.ErrorString(code), message)

	copy(prgrm.Memory, machine.MachineMemory[:ir.ImageDataEnd(image)])
	depth := machine.MachineDepth
	for i := uint32(0); i < depth; i++ {
		frame := &machine.MachineFrames[i]
		prgrm.CallStack[i].Operator = &prgrm.CXFunctions[frame.FrameFunction]
		prgrm.CallStack[i].FramePointer = types.Pointer(frame.FramePointer)
	}
	prgrm.CallStack[depth].Operator = &prgrm.CXFunctions[machine.MachineFunction]
	prgrm.CallStack[depth].FramePointer = types.Pointer(machine.MachineFramePointer)
	prgrm.CallCounter = types.Pointer(depth)
	prgrm.Stack.Pointer = types.Pointer(machine.MachineStackPointer)
	PrinterStackPrint(prgrm)

	if globals.DBG_GOLANG_STACK_TRACE {
		debug.PrintStack()
	}
}
