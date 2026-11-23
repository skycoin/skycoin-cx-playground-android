package ast

import (
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	"github.com/skycoin/skycoin-cx/t2-globals" // thin: only FoundCompileErrors; core globals, keep
)

// CompileErrorCode names the message format of a compile error. The codes
// take the compiler block of docs/ErrorHandling.md, and the printer package
// holds the format of each one in a table indexed by the code.
type CompileErrorCode int32

const (
	CompileErrorCodeNil CompileErrorCode = 7000 + iota
	// CompileErrorCodeMessage prints its operand, the text of a Go error
	// or a lexer message, which has no code of its own.
	CompileErrorCodeMessage
	CompileErrorCodeAssignOutputlessOperator
	CompileErrorCodeAssignSymbolType
	CompileErrorCodeAssignValueType
	CompileErrorCodeBuiltinPackageNotImported
	CompileErrorCodeFunctionInputType
	CompileErrorCodeFunctionMissing
	CompileErrorCodeFunctionReceiverType
	// CompileErrorCodeRedeclaredBlock is a package level name declared twice.
	// Its operands are the name and the file, line and column of the first
	// declaration.
	CompileErrorCodeRedeclaredBlock
	CompileErrorCodeFunctionReturnCount
	CompileErrorCodeIdentifierMissing
	CompileErrorCodeIndexType
	CompileErrorCodeInputTypesNotEqual
	CompileErrorCodeInvalidExpression
	CompileErrorCodeInvalidIndexing
	CompileErrorCodeInvalidIndirection
	CompileErrorCodeLoopConditionNoValue
	CompileErrorCodeNoFields
	CompileErrorCodeNoPackageDefined
	CompileErrorCodeOpcodeRenamed
	CompileErrorCodeOpcodeRenamedImport
	CompileErrorCodeOperatorArgumentTypes
	CompileErrorCodeOperatorInputCount
	CompileErrorCodeOperatorOutputCount
	CompileErrorCodePackageIdentifierMissing
	CompileErrorCodePointerOrderingOperator
	CompileErrorCodePrimitiveFieldAccess
	CompileErrorCodeRedeclared
	CompileErrorCodeStrArithmeticOperator
	CompileErrorCodeStrConcatenation
	CompileErrorCodeStrEqualityOperator
	CompileErrorCodeStrOrderingOperator
	CompileErrorCodeStructFieldDuplicate
	CompileErrorCodeStructFieldMultiplyDefined
	CompileErrorCodeStructLiteralFieldType
	CompileErrorCodeMethodMissing
	CompileErrorCodeDeclarationTooLarge
	CompileErrorCodeStructRecursive
	CompileErrorCodeLoopJumpOutsideLoop
	CompileErrorCodeSliceArgumentType
	CompileErrorCodeGotoLabelMissing
	CompileErrorCodeSliceElementType
	CompileErrorCodeCallResultCall
	CompileErrorCodeMethodResultMissing
	CompileErrorCodeAssignmentCount
	CompileErrorCodeArrayLiteralLength
	CompileErrorCodeStructLiteralFieldRepeated
	CompileErrorCodeImportGuiBuildMissing
	CompileErrorCodeLiteralElementType
	CompileErrorCodeStructLiteralFieldNames
	CompileErrorCodeSliceLiteralElementType
	// CompileErrorCodeSyntax is a syntax error, its operand the text of
	// CompileErrorSyntaxText.
	CompileErrorCodeSyntax
	CompileErrorCodeDereferenceNonPointer
	// CompileErrorCodeMethodRedeclared has the operands of
	// CompileErrorCodeRedeclaredBlock.
	CompileErrorCodeMethodRedeclared
	// CompileErrorCodeImportFailed is an import of a package that cannot be
	// loaded. Its operands are the import path and the reason.
	CompileErrorCodeImportFailed
	// CompileErrorCodeMethodReceiversMultiple is a method with more than one
	// receiver, at the second.
	CompileErrorCodeMethodReceiversMultiple
	// CompileErrorCodeMethodPointerValue is a pointer method called on a value
	// that is not addressable, such as T{}.p(). Its operands are the method and
	// the type.
	CompileErrorCodeMethodPointerValue
	// CompileErrorCodeArrayLiteralElementType is an array literal element of
	// the wrong type, at the element. Its operands are the two types.
	CompileErrorCodeArrayLiteralElementType
	// CompileErrorCodeReturnValueType is a value of the wrong type returned as
	// the unnamed result of func f() T. Its operands are the two types.
	CompileErrorCodeReturnValueType
	// CompileErrorCodeGlobalTypeUnknown is a global declared without a type
	// whose value has no type it can take. Its operand is the global.
	CompileErrorCodeGlobalTypeUnknown
	// CompileErrorCodeInitializationCycle is a global whose initializer refers
	// to itself through other globals or functions. Its operands are the
	// global and the chain of references.
	CompileErrorCodeInitializationCycle
	// CompileErrorCodeEnd is one past the last code.
	CompileErrorCodeEnd
)

// CompileErrorOperands are the operands of a compile error, each a
// stringpool.StringGid or an integer as the format of its code says.
type CompileErrorOperands [4]int64

// CompileErrorRecord is a compile error as data, see #413. The compiler
// records it without building text and the printer package prints it. A
// column of 0 is unknown.
type CompileErrorRecord struct {
	CompileErrorRecordCode     CompileErrorCode
	CompileErrorRecordFile     SourceFileGid
	CompileErrorRecordLine     int
	CompileErrorRecordColumn   int
	CompileErrorRecordOperands CompileErrorOperands
}

// CompileErrorList holds the compile errors of a program in the order they
// were found. CompileErrorListPrinted counts the records already printed.
// The token fields hold the position of the token the parser read last.
type CompileErrorList struct {
	CompileErrorListRecords     []CompileErrorRecord
	CompileErrorListPrinted     int
	CompileErrorListTokenFile   SourceFileGid
	CompileErrorListTokenLine   int
	CompileErrorListTokenColumn int
	// CompileErrorListPreliminary counts the records of the preliminary stage,
	// which the body pass does not record again.
	CompileErrorListPreliminary int
	// CompileErrorListImportsFailed names the packages whose import failed,
	// whose members report no error of their own.
	CompileErrorListImportsFailed []stringpool.StringGid
}

// CompileErrorListInit sets up an empty list with room for capacity records.
func (list *CompileErrorList) CompileErrorListInit(capacity int) {
	list.CompileErrorListRecords = make([]CompileErrorRecord, 0, capacity)
	list.CompileErrorListPrinted = 0
	list.CompileErrorListTokenFile, list.CompileErrorListTokenLine, list.CompileErrorListTokenColumn = SourceFileGidNil, 0, 0
	list.CompileErrorListPreliminary = 0
	list.CompileErrorListImportsFailed = nil
}

// CXProgramCompileErrorTokenSet records the position of the token the
// parser is at, the column of the errors and debug records made on its line.
func (prgrm *CXProgram) CXProgramCompileErrorTokenSet(file SourceFileGid, line int, column int) {
	if prgrm == nil {
		// A lexer test runs without a program.
		return
	}
	list := &prgrm.CxIrState.CompileErrorList
	list.CompileErrorListTokenFile, list.CompileErrorListTokenLine, list.CompileErrorListTokenColumn = file, line, column
}

// CXProgramCompileErrorColumn returns the column of the token the parser is
// at when it is on line of file, and 0 otherwise.
func (prgrm *CXProgram) CXProgramCompileErrorColumn(file SourceFileGid, line int) int {
	list := &prgrm.CxIrState.CompileErrorList
	if list.CompileErrorListTokenFile != file || list.CompileErrorListTokenLine != line {
		return 0
	}
	return list.CompileErrorListTokenColumn
}

// CXProgramCompileErrorAdd records a compile error at file and line, at the
// column of the token the parser is at when it is on that line.
func (prgrm *CXProgram) CXProgramCompileErrorAdd(code CompileErrorCode, file SourceFileGid, line int, operands CompileErrorOperands) {
	prgrm.CXProgramCompileErrorAddPosition(code, file, line, prgrm.CXProgramCompileErrorColumn(file, line), operands)
}

// CXProgramCompileErrorAddPosition records a compile error at file, line and
// column, and records that the program has a compile error.
func (prgrm *CXProgram) CXProgramCompileErrorAddPosition(code CompileErrorCode, file SourceFileGid, line int, column int, operands CompileErrorOperands) {
	globals.FoundCompileErrors = true
	list := &prgrm.CxIrState.CompileErrorList
	for index := range list.CompileErrorListRecords[:list.CompileErrorListPreliminary] {
		record := &list.CompileErrorListRecords[index]
		if record.CompileErrorRecordCode == code && record.CompileErrorRecordFile == file && record.CompileErrorRecordLine == line && record.CompileErrorRecordOperands == operands {
			return
		}
	}
	list.CompileErrorListRecords = append(list.CompileErrorListRecords, CompileErrorRecord{
		CompileErrorRecordCode:     code,
		CompileErrorRecordFile:     file,
		CompileErrorRecordLine:     line,
		CompileErrorRecordColumn:   column,
		CompileErrorRecordOperands: operands,
	})
}

// CXProgramCompileErrorText interns text for a StringGid operand. Only the
// error path calls it, for names that are not in the pool already.
func (prgrm *CXProgram) CXProgramCompileErrorText(text string) int64 {
	return int64(prgrm.CxIrState.StringPool.StringPoolIntern([]byte(text)))
}

// CXProgramCompileErrorExit prints the compile errors not printed yet and
// ends the process, for a compile error the compiler cannot go on after.
func (prgrm *CXProgram) CXProgramCompileErrorExit() {
	if prgrm.CompileErrorPrint != nil {
		prgrm.CompileErrorPrint(prgrm)
	}
	os.Exit(types.CX_COMPILATION_ERROR)
}

// CXArgumentDebugFileGidLine returns the source file and line of the debug
// record gid, SourceFileGidNil and -1 for CXArgumentDebugGidNil.
func CXArgumentDebugFileGidLine(prgrm *CXProgram, gid CXArgumentDebugGid) (SourceFileGid, int) {
	file, line, _ := CXArgumentDebugPosition(prgrm, gid)
	return file, line
}

// CXArgumentDebugPosition returns the source file, line and column of the
// debug record gid, SourceFileGidNil, -1 and 0 for CXArgumentDebugGidNil.
func CXArgumentDebugPosition(prgrm *CXProgram, gid CXArgumentDebugGid) (SourceFileGid, int, int) {
	if gid == CXArgumentDebugGidNil {
		return SourceFileGidNil, -1, 0
	}
	details := prgrm.CxIrState.CXArgumentDebugMng.CXArgumentDebugGet(gid)
	return details.FileName, details.FileLine, details.FileColumn
}

// CXProgramCompileErrorAddDebug records a compile error at the source file,
// line and column of the debug record details.
func (prgrm *CXProgram) CXProgramCompileErrorAddDebug(code CompileErrorCode, details CXArgumentDebugGid, operands CompileErrorOperands) {
	file, line, column := CXArgumentDebugPosition(prgrm, details)
	prgrm.CXProgramCompileErrorAddPosition(code, file, line, column, operands)
}

// CXProgramImportFailedAdd records that the import of the package name failed.
func (prgrm *CXProgram) CXProgramImportFailedAdd(name stringpool.StringGid) {
	list := &prgrm.CxIrState.CompileErrorList
	list.CompileErrorListImportsFailed = append(list.CompileErrorListImportsFailed, name)
}

// CXProgramImportFailed reports whether the import of the package name failed.
func (prgrm *CXProgram) CXProgramImportFailed(name stringpool.StringGid) bool {
	for _, failed := range prgrm.CxIrState.CompileErrorList.CompileErrorListImportsFailed {
		if failed == name {
			return true
		}
	}
	return false
}

// CXProgramCompileErrorAddLine records a compile error at file and line with
// no column, for a check after a function is parsed, when the token the
// parser is at is past the function.
func (prgrm *CXProgram) CXProgramCompileErrorAddLine(code CompileErrorCode, file SourceFileGid, line int, operands CompileErrorOperands) {
	prgrm.CXProgramCompileErrorAddPosition(code, file, line, 0, operands)
}
