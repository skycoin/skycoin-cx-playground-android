// Package core is the minimum interpreter core that cx build puts in front of
// a serialized program: the opcode set it links, the trailer that marks the
// appended program, and the executors. It uses no module outside the Go
// standard library and has no parser. A core that links the cipher library
// links the cipher package of Skycoin with it.
package core

import (
	"sort"

	"github.com/skycoin/skycoin-cx/t0-cx-core-assert"
	"github.com/skycoin/skycoin-cx/t1-constants"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
	// GCREACH: t3-ast owns the heap and the GC roots. core builds the program and
	// allocates nothing, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-cx-runtime-printer"
	"github.com/skycoin/skycoin-cx/t4-regexp"
)

// CoreRegister adds the native packages of the core to prgrm, registers the
// system libraries that libraryRegisters register and sets the opcode switch
// to CoreOpcodeDispatch. It leaves out every other library (t5-cxos,
// t5-cxnet, t5-cipher) and the opcodes that need a module outside the
// standard library (t6-cxfx).
func CoreRegister(prgrm *ast.CXProgram, libraryRegisters ...func(prgrm *ast.CXProgram)) {
	ast.OpcodeDispatch = CoreOpcodeDispatch
	printer.PrinterCompileErrorsRegister(prgrm)
	regexp.RegisterPackage(prgrm)
	for _, register := range libraryRegisters {
		register(prgrm)
	}
}

// CoreProgramMake returns an empty program to register the libraries in. A
// program to run comes from ast.Deserialize instead.
func CoreProgramMake() *ast.CXProgram {
	state := &ast.CxIrState{}
	state.CxIrStateInit()
	return &ast.CXProgram{
		Packages:       make(map[stringpool.StringGid]ast.CXPackageIndex),
		CurrentPackage: -1,
		Version:        constants.LanguageVersionCurrent,
		CxIrState:      state,
	}
}

// CoreOpcodeLinked reports whether the core runs opcode code: LibraryCall,
// which the loop runs, or an opcode with a handler outside t6-cxfx that the
// build tags link. The core is built with the tags of cx, so cx can ask.
func CoreOpcodeLinked(code int) bool {
	if code == constants.OP_LIBRARY_CALL {
		return true
	}
	return constants.OpcodeHandled(code) && constants.OpcodeLinked(code) && constants.OpcodeSignatureOf(code).OpcodeSignatureGroup != constants.OpcodeGroupCxfx
}

// CoreOpcodesUsed returns the sorted numbers of the opcodes prgrm can call,
// those of its built-in functions.
func CoreOpcodesUsed(prgrm *ast.CXProgram) []int {
	seen := map[int]bool{}
	var used []int
	for index := range prgrm.CXFunctions {
		code := prgrm.CXFunctions[index].AtomicOPCode
		if code != 0 && !seen[code] {
			seen[code] = true
			used = append(used, code)
		}
	}
	sort.Ints(used)
	return used
}

// CoreOpcodesMissing returns the names of the opcodes prgrm uses that the
// core does not link, sorted by number.
func CoreOpcodesMissing(prgrm *ast.CXProgram) []string {
	var missing []string
	for _, code := range CoreOpcodesUsed(prgrm) {
		if !CoreOpcodeLinked(code) {
			missing = append(missing, CoreOpcodeName(code))
		}
	}
	return missing
}

// CoreOpcodeName returns the name of opcode number code in
// constants.OpcodeLifecycleTable.
func CoreOpcodeName(code int) string {
	row, found := constants.OpcodeLifecycleLiveByNumber(code)
	assert.AssertPreconditionTrue(found, "opcode number without a live row")
	return row.Name
}

// CoreLibrariesEnabledResolve applies constants.LibraryPolicyEnabledWhenImported
// to prgrm: when it is true, every library the program calls is enabled.
func CoreLibrariesEnabledResolve(prgrm *ast.CXProgram) {
	if !constants.LibraryPolicyEnabledWhenImported {
		return
	}
	for library, used := range prgrm.ProgramLibrariesUsed {
		if used {
			prgrm.ProgramLibrariesEnabled[library] = true
		}
	}
}

// CoreLibrariesMissing returns the names of the libraries enabled for prgrm
// that this binary does not link, in number order.
func CoreLibrariesMissing(prgrm *ast.CXProgram) []string {
	var missing [constants.LibraryCount]bool
	for library, enabled := range prgrm.ProgramLibrariesEnabled {
		missing[library] = enabled && !ast.LibraryLinked(library)
	}
	return CoreLibraryNames(missing)
}

// CoreLibraryNames returns the names of the libraries libraries marks, in
// number order.
func CoreLibraryNames(libraries [constants.LibraryCount]bool) []string {
	var names []string
	for library, marked := range libraries {
		if marked {
			names = append(names, constants.LibraryNameOf(library))
		}
	}
	return names
}
