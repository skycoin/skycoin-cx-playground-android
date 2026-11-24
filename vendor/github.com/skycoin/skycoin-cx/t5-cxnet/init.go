package cxnet

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t1-netbatch"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only In, Out, RegisterLibraryFunction, RegisterLibraryFunctionVm; registration API, keep
)

// RegisterPackage adds the net package to prgrm and registers the net
// library, the functions of sys/net, see docs/Networking.md.
func RegisterPackage(prgrm *ast.CXProgram) {
	netbatch.NetPrepare()
	ast.LibraryRunStarts[constants.LibraryNumberNet] = LibraryNetRunStart

	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now a constructor call, site S1427.
	netPkg := ast.MakePackage(prgrm.CXProgramNameIntern("net"))
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena insert, site S1428.
	prgrm.AddPackage(netPkg)

	library := constants.LibraryNumberNet
	i32, i64, str, boolean := ast.ConstCxArg_I32, ast.ConstCxArg_I64, ast.ConstCxArg_STR, ast.ConstCxArg_BOOL
	// A byte buffer is a ui8 array of any length, read or filled in place.
	buffer := ast.ConstCxArg_UND_TYPE
	opcodes.RegisterLibraryFunction(prgrm, library, "net.Update", opNetUpdate, opcodes.In(i32), opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.ReadyCount", opNetReadyCount, nil, opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.ReadyNext", opNetReadyNext, nil, opcodes.Out(i32, i32, i32, i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.Clock", opNetClock, nil, opcodes.Out(i64))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.Read", opNetRead, opcodes.In(i32, i32, buffer, i32), opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.Write", opNetWrite, opcodes.In(i32, buffer, i32), opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.Close", opNetClose, opcodes.In(i32), opcodes.Out(boolean))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.TimeoutSet", opNetTimeoutSet, opcodes.In(i32, i32), opcodes.Out(boolean))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.TCPListen", opNetTCPListen, opcodes.In(str, i32), opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.TCPDial", opNetTCPDial, opcodes.In(str, i32), opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.UDPListen", opNetUDPListen, opcodes.In(str, i32), opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.UDPDial", opNetUDPDial, opcodes.In(str, i32), opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.UDPReadFrom", opNetUDPReadFrom, opcodes.In(i32, i32, buffer, i32), opcodes.Out(i32, i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.UDPWriteTo", opNetUDPWriteTo, opcodes.In(i32, i32, buffer, i32), opcodes.Out(i32))
	opcodes.RegisterLibraryFunction(prgrm, library, "net.UDPPeerResolve", opNetUDPPeerResolve, opcodes.In(str, i32), opcodes.Out(i32))

	// Every function has a vm form. A str input is read from the data
	// segment and a buffer is the address and size the lowering passes.
	opcodes.RegisterLibraryFunctionVm(library, "net.Update", libraryNetUpdateVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.ReadyCount", libraryNetReadyCountVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.ReadyNext", libraryNetReadyNextVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.Clock", libraryNetClockVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.Read", libraryNetReadVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.Write", libraryNetWriteVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.Close", libraryNetCloseVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.TimeoutSet", libraryNetTimeoutSetVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.TCPListen", libraryNetTCPListenVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.TCPDial", libraryNetTCPDialVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.UDPListen", libraryNetUDPListenVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.UDPDial", libraryNetUDPDialVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.UDPReadFrom", libraryNetUDPReadFromVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.UDPWriteTo", libraryNetUDPWriteToVm)
	opcodes.RegisterLibraryFunctionVm(library, "net.UDPPeerResolve", libraryNetUDPPeerResolveVm)
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
