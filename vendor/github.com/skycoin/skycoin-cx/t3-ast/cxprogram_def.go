package ast

import (
	"bufio"
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

// CXProgram is used to represent a full CX program.
//
// It is the root data structure for the declarations of all functions,
// variables and data structures.
type CXProgram struct {
	// Metadata
	//Remove Path //moved to cx/globals
	//Path string // Path to the CX project in the filesystem

	Stack StackSegmentStruct
	Data  DataSegmentStruct
	Heap  HeapSegmentStruct

	// Contents
	Packages map[stringpool.StringGid]CXPackageIndex // Packages in a CX program

	// Runtime information
	ProgramInput []CXArgumentIndex // OS input arguments
	//ProgramOutput []*CXArgument // outputs to the OS
	Memory []byte // Used when running the program

	// OBJMODEL: []CXCallGid or CXCallGid, phase 5 #367.
	// Now objects held by value, site S0186.
	CallStack   []CXCall      // Collection of function calls. Call stack is not in CX Memory.
	CallCounter types.Pointer // What function call is the currently being executed in the CallStack
	Terminated  bool          // Utility field for the runtime. Indicates if a CX program has already finished or not.
	Version     int64         // Language version, constants.LanguageVersionCurrent when built.

	// ProgramLibrariesUsed marks, by library number, the system libraries the
	// program calls. ProgramLibrariesEnabled marks the ones this run may call,
	// set from --enable, see constants.LibraryPolicyEnabledWhenImported.
	ProgramLibrariesUsed    [constants.LibraryCount]bool
	ProgramLibrariesEnabled [constants.LibraryCount]bool

	// RuntimeError is the runtime error that ended the old loop, printed by
	// the printer package once the loop has returned.
	RuntimeError RuntimeErrorRecord

	// CompileErrorPrint prints the compile errors in CxIrState that are not
	// printed yet. The printer package sets it, see #413.
	CompileErrorPrint func(prgrm *CXProgram)

	// ProgramSandboxRoot is the directory every program file opcode resolves
	// in, opened once at run start, see #433. Nil means file opcodes fail.
	ProgramSandboxRoot *os.Root

	// ProgramInputValues and ProgramOutputValues hold the operands of the
	// native opcode the old loop runs, sized by RunBuffersPrepare (#407).
	ProgramInputValues  []CXValue
	ProgramOutputValues []CXValue

	// ProgramFormatBuffer is the scratch the print, printf and str conversion
	// opcodes format into, kept between them so they do not allocate (#412).
	ProgramFormatBuffer []byte

	// ProgramCallbackValues hold the operands of the nested loops Callback
	// runs, one set per depth of nesting, and ProgramCallbackDepth counts the
	// loops running. RunBuffersPrepare sizes the first set (#407).
	ProgramCallbackValues []CXCallbackValues
	ProgramCallbackDepth  int

	// ProgramStdin is the one reader read() takes lines from for the whole run,
	// set by ProgramStdinInit, see skycoin/cx#110. Nil reads as end of input.
	ProgramStdin *bufio.Reader

	// Used by the REPL and cxgo
	CurrentPackage CXPackageIndex // Represents the currently active package in the REPL or when parsing a CX file.
	ProgramError   error

	// For new CX AST arrays
	CXAtomicOps []CXAtomicOperator
	CXPackages  []CXPackage
	CXFunctions []CXFunction
	CXStructs   []CXStruct
	// Then reference the package of function by CxFunction id

	// CxIrState owns this program's IR objects. The program reaches it here until
	// phase 5 of CXProgram passes the state itself, see docs/ObjectModel.md.
	CxIrState *CxIrState

	// For Initializers
	// OBJMODEL: []CXExpressionGid or CXExpressionGid, phase 5 #347.
	// Now objects held by value, site S0187.
	SysInitExprs []CXExpression

	// ImportPositions is where each package imports another, for the shadowing
	// errors of #486. Compiler only, never serialized.
	ImportPositions map[CXProgramImportKey]CXProgramImportPosition

	// ImportScoped holds the imports into scope, import . "pkg" and the
	// others of #487. Compiler only, never serialized.
	ImportScoped []CXProgramImportScoped
}
