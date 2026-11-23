//go:build !nocipher

package cipher

import (
	"github.com/skycoin/skycoin-cx/t0-types"     // thin: only Pointer, UI8; no depth gain
	"github.com/skycoin/skycoin-cx/t1-constants" // thin: only DECL_ARRAY; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only Out, RegisterFunction; registration API, keep
)

func RegisterPackage(prgrm *ast.CXProgram) {
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now a constructor call, site S1359.
	cipherPkg := ast.MakePackage(prgrm.CXProgramNameIntern("cipher"))
	// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
	// Now a constructor call, site S1360.
	pubkeyStrct := ast.MakeStruct(prgrm.CXProgramNameIntern("PubKey"))
	// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
	// Now a constructor call, site S1361.
	seckeyStrct := ast.MakeStruct(prgrm.CXProgramNameIntern("SecKey"))

	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena insert, site S1362.
	pkgIdx := prgrm.AddPackage(cipherPkg)
	// OBJMODEL: state.CXPackageMng.CXPackageGet(gid), phase 5 #362.
	// Now an arena getter, site S1363.
	cPkg, _ := prgrm.GetPackageFromArray(pkgIdx)

	// PubKey
	_, pubkeyFld := ast.MakeArgument(prgrm, prgrm.CXProgramNameIntern("PubKey"), ast.SourceFileGidNil, -1)
	pubkeyFld.SetType(types.UI8).SetPackage(cPkg)
	pubkeyFld.DeclarationSpecifiers = append(pubkeyFld.DeclarationSpecifiers, constants.DECL_ARRAY)
	// pubkeyFld.IsArray = true
	pubkeyFld.Lengths = []types.Pointer{33} // Yes, PubKey is 33 bytes long.
	// pubkeyFld.TotalSize = 33                // 33 * 1 byte (ui8)

	// SecKey
	_, seckeyFld := ast.MakeArgument(prgrm, prgrm.CXProgramNameIntern("SecKey"), ast.SourceFileGidNil, -1)
	seckeyFld.SetType(types.UI8).SetPackage(cPkg)
	seckeyFld.DeclarationSpecifiers = append(seckeyFld.DeclarationSpecifiers, constants.DECL_ARRAY)
	// seckeyFld.IsArray = true
	seckeyFld.Lengths = []types.Pointer{32} // Yes, SecKey is 32 bytes long.
	// seckeyFld.TotalSize = 33                // 33 * 1 byte (ui8)

	pubkeyStrct.AddField(prgrm, pubkeyFld)
	seckeyStrct.AddField(prgrm, seckeyFld)

	cPkg.AddStruct(prgrm, pubkeyStrct)
	cPkg.AddStruct(prgrm, seckeyStrct)

	opcodes.RegisterFunction(prgrm, "cipher.GenerateKeyPair", opCipherGenerateKeyPair, nil,
		// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
		// Now a constructor call, site S1366 S1367.
		opcodes.Out(ast.MakeStructParameter(prgrm, "cipher", "PubKey", "pubKey"), ast.MakeStructParameter(prgrm, "cipher", "SecKey", "sec")))
}
