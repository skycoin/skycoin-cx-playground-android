//go:build !nocipher

package cipher

import (
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only LibraryLink; registration API, keep
)

// RegisterPackage adds the cipher package to prgrm and links the cipher
// library, the functions of sys/cipher, see docs/CipherLibrary.md.
func RegisterPackage(prgrm *ast.CXProgram) {
	cipherStatePrepare()

	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now a constructor call, site S1359.
	cipherPkg := ast.MakePackage(prgrm.CXProgramNameIntern("cipher"))
	// OBJMODEL: state.CXPackageMng.CXPackageAlloc(), phase 4 #361.
	// Now an arena insert, site S1362.
	prgrm.AddPackage(cipherPkg)

	opcodes.LibraryLink(constants.LibraryNumberCipher, LibraryCipherDispatch, LibraryCipherDispatchVm)
}
