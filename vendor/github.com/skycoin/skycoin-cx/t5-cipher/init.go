//go:build !nocipher

package cipher

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only In, Out, RegisterLibraryFunction, RegisterLibraryFunctionVm; registration API, keep
)

// RegisterPackage adds the cipher package to prgrm and registers the cipher
// library, the functions of sys/cipher, see docs/CipherLibrary.md.
func RegisterPackage(prgrm *ast.CXProgram) {
	cipherStatePrepare()

	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now a constructor call, site S1359.
	cipherPkg := ast.MakePackage(prgrm.CXProgramNameIntern("cipher"))
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena insert, site S1362.
	prgrm.AddPackage(cipherPkg)

	library := constants.LibraryNumberCipher
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.HashOfBytes", opCipherHashOfBytes, opcodes.In(ast.Slice(types.UI8)), opcodes.Out(ast.Slice(types.UI8)))
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.HashStateNew", opCipherHashStateNew, nil, opcodes.Out(ast.ConstCxArg_I32))
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.HashStateWrite", opCipherHashStateWrite, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI8)), nil)
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.HashStateSum", opCipherHashStateSum, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.Slice(types.UI8)))
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.HashStateRelease", opCipherHashStateRelease, opcodes.In(ast.ConstCxArg_I32), nil)
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.SignatureVerify", opCipherSignatureVerify, opcodes.In(ast.Slice(types.UI8), ast.Slice(types.UI8), ast.Slice(types.UI8)), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.SignatureRecoverPubKey", opCipherSignatureRecoverPubKey, opcodes.In(ast.Slice(types.UI8), ast.Slice(types.UI8)), opcodes.Out(ast.Slice(types.UI8), ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.SignatureSign", opCipherSignatureSign, opcodes.In(ast.Slice(types.UI8), ast.Slice(types.UI8)), opcodes.Out(ast.Slice(types.UI8), ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, library, "cipher.KeyPairGenerate", opCipherKeyPairGenerate, nil, opcodes.Out(ast.Slice(types.UI8), ast.Slice(types.UI8)))

	// The vm has no heap, so only the functions without a []ui8 have a vm form.
	opcodes.RegisterLibraryFunctionVm(library, "cipher.HashStateNew", libraryCipherHashStateNewVm)
	opcodes.RegisterLibraryFunctionVm(library, "cipher.HashStateRelease", libraryCipherHashStateReleaseVm)
}
