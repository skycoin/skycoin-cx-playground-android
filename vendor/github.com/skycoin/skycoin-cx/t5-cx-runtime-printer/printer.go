// Package printer produces the text of runtime errors once a run has
// returned, from the data the old loop and vm record, see #413.
package printer

import (
	"fmt"
	"strconv"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes"
)

// PrinterErrorHeader returns "error: file:line", the start of an error.
func PrinterErrorHeader(fileName string, lineNumber int) string {
	return "error: " + fileName + ":" + strconv.FormatInt(int64(lineNumber), 10)
}

// PrinterRuntimeErrorPrint prints the runtime error recorded in
// prgrm.RuntimeError and returns its exit code. The record must be raised.
func PrinterRuntimeErrorPrint(prgrm *ast.CXProgram) int {
	record := &prgrm.RuntimeError
	call := prgrm.CallStack[record.RuntimeErrorRecordCall]
	cxLine, _ := prgrm.GetPreviousCXLine(call.Operator.Expressions, record.RuntimeErrorRecordLine)
	fileName := prgrm.CXProgramSourceFileText(cxLine.FileName)

	if record.RuntimeErrorRecordAssertLine {
		fmt.Printf("%s : %d, %s\n", fileName, cxLine.LineNumber, printerHeapBytes(prgrm, record.RuntimeErrorRecordAssertMessage))
	}

	fmt.Printf("%s, %s, %v", PrinterErrorHeader(fileName, cxLine.LineNumber), ast.ErrorString(record.RuntimeErrorRecordCode), record.RuntimeErrorRecordPanicValue)

	if record.RuntimeErrorRecordStackPrint {
		PrinterStackPrint(prgrm)
	}
	return record.RuntimeErrorRecordCode
}

// printerHeapBytes returns the bytes of the CX string at heapOffset, nil
// for the empty string, as types.Read_obj reads them.
func printerHeapBytes(prgrm *ast.CXProgram, heapOffset types.Pointer) []byte {
	if heapOffset != 0 && heapOffset.IsValid() {
		return types.Read_obj_data(prgrm.Memory, heapOffset)
	}
	return nil
}

// PrinterOpcodesRegister registers the debug opcode, which prints the call
// stack, on the global opcode tables.
func PrinterOpcodesRegister(prgrm *ast.CXProgram) {
	opcodes.RegisterOpCode(prgrm, constants.OP_DEBUG_PRINT_STACK, "debug", printerOpDebugPrintStack, nil, nil)
}

func printerOpDebugPrintStack(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	PrinterStackPrint(prgrm)
}
