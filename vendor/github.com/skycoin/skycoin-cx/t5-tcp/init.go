package tcp

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only In, Out, RegisterLibraryFunction; registration API, keep
)

// RegisterPackage adds the tcp package to prgrm and registers the net
// library, the functions of sys/tcp.
func RegisterPackage(prgrm *ast.CXProgram) {

	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now a constructor call, site S1427.
	netPkg := ast.MakePackage(prgrm.CXProgramNameIntern("tcp"))
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena insert, site S1428.
	pkgIdx := prgrm.AddPackage(netPkg)
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S1429.
	netPkg, _ = prgrm.GetPackageFromArray(pkgIdx)

	// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
	// Now a constructor call, site S1430.
	dialerStrct := ast.MakeStruct(prgrm.CXProgramNameIntern("Dialer"))
	netPkg.AddStruct(prgrm, dialerStrct)

	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberNet, "tcp.Dial", opTCPDial, opcodes.In(ast.ConstCxArg_STR, ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_STR))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberNet, "tcp.Listen", opTCPListen, opcodes.In(ast.ConstCxArg_STR, ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_STR))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberNet, "tcp.Accept", opTCPAccept, opcodes.In(ast.ConstCxArg_STR, ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_STR))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberNet, "tcp.Close", opTCPClose, nil, nil)
	opcodes.RegisterLibraryFunctionVm(constants.LibraryNumberNet, "tcp.Close", libraryNetCloseVm)
}
