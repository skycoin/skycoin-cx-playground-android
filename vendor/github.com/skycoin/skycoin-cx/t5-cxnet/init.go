package cxnet

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t1-netbatch"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only LibraryLink; registration API, keep
)

// RegisterPackage adds the net package to prgrm and links the net
// library, the functions of sys/net, see docs/Networking.md. Every function
// has a vm form, which reads a str input from the data segment and takes a
// buffer as the address and size the lowering passes.
func RegisterPackage(prgrm *ast.CXProgram) {
	netbatch.NetPrepare()
	ast.LibraryRunStarts[constants.LibraryNumberNet] = LibraryNetRunStart

	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now a constructor call, site S1427.
	netPkg := ast.MakePackage(prgrm.CXProgramNameIntern("net"))
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena insert, site S1428.
	prgrm.AddPackage(netPkg)

	opcodes.LibraryLink(constants.LibraryNumberNet, LibraryNetDispatch, LibraryNetDispatchVm)
}

// The constant codes of the net.Trigger* and net.Error* constants.
const (
	constNetTriggerFirst = iota + 0x2FFFF
	constNetErrorFirst   = constNetTriggerFirst + int(netbatch.NetTriggerCount)
)

// InitConstants registers the net.Trigger* and net.Error* constants through
// constantAdd, actions.AddConstI32 in cx, which keeps the compiler out of
// the cx build core.
func InitConstants(constantAdd func(code int, name string, value int32)) {
	triggers := [netbatch.NetTriggerCount]string{"None", "Data", "Accepted", "Connected", "Listening", "Closed", "Error", "Timeout", "Dropped"}
	for value, name := range triggers {
		constantAdd(constNetTriggerFirst+value, "net.Trigger"+name, int32(value))
	}
	errors := [netbatch.NetErrorCount]string{"None", "Other", "Refused", "Reset", "Timeout", "Address", "AddressInUse", "SocketTableFull"}
	for value, name := range errors {
		constantAdd(constNetErrorFirst+value, "net.Error"+name, int32(value))
	}
}

// netPointerSize is the size of a pointer in a vm block.
const netPointerSize = uint32(types.POINTER_SIZE)

// LibraryNetMode and LibraryNetRecordPath say where the batches of the next
// run come from, set by cx --net-record and --net-replay.
// VIOLATION: state outside State. Owner: t5-cxnet; moves to a per program state with the cipher tables, see D45.
var (
	LibraryNetMode       = netbatch.NetModeLive
	LibraryNetRecordPath string
)

// LibraryNetRunStart closes what an earlier run left open and starts the
// run in LibraryNetMode.
func LibraryNetRunStart() error {
	return netbatch.NetRunStart(LibraryNetMode, LibraryNetRecordPath)
}
