package actions

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// gcRootsDeclare adds a new local that holds a pointer, slice or str, also
// an escaped one, to the GC roots of fn with its declared type.
func gcRootsDeclare(prgrm *ast.CXProgram, fn *ast.CXFunction, typeSig *ast.CXTypeSignature, sym *ast.CXArgument) {
	offset := typeSig.Offset
	specs, typ, strct, lengths := ast.SignatureGCDeclaration(prgrm, typeSig)
	if sym != nil {
		offset = sym.Offset
		specs, typ, strct, lengths = ast.CXArgumentGCDeclaration(sym)
	}
	if offset >= prgrm.Stack.Size && !ast.OffsetEscaped(offset) {
		return
	}
	if !gcValueReferences(prgrm, specs, typ, strct, 0) {
		return
	}
	gcRootAdd(fn, offset, specs, typ, strct, lengths)
}

// gcRootAdd adds the local at offset to fn.ListOfSlots.
func gcRootAdd(fn *ast.CXFunction, offset types.Pointer, specs []int, typ types.Code, strct *ast.CXStruct, lengths []types.Pointer) {
	fn.ListOfSlots = append(fn.ListOfSlots, ast.CXFunctionSlot{
		CXFunctionSlotOffset:     offset,
		CXFunctionSlotLengths:    lengths,
		CXFunctionSlotType:       typ,
		CXFunctionSlotSpecifiers: specs,
		CXFunctionSlotStruct:     strct,
	})
}

// gcRootAddEscapeSlot adds the frame slot of an escaped local, which holds
// its heap object, to fn.ListOfSlots.
func gcRootAddEscapeSlot(fn *ast.CXFunction, offset types.Pointer) {
	gcRootAdd(fn, offset, []int{constants.DECL_BASIC, constants.DECL_POINTER}, types.UNDEFINED, nil, nil)
}

// gcPointerDepthLimit bounds the struct nesting gcValueReferences follows.
const gcPointerDepthLimit = 16

// gcValueReferences reports whether a value of declaration specifiers specs
// holds a pointer, slice or str.
func gcValueReferences(prgrm *ast.CXProgram, specs []int, typ types.Code, strct *ast.CXStruct, depth int) bool {
	if depth > gcPointerDepthLimit || len(specs) == 0 {
		return false
	}
	switch specs[len(specs)-1] {
	case constants.DECL_POINTER, constants.DECL_SLICE:
		return true
	case constants.DECL_ARRAY:
		return gcValueReferences(prgrm, specs[:len(specs)-1], typ, strct, depth+1)
	}
	if strct == nil {
		return typ == types.STR
	}
	for _, fieldIdx := range strct.Fields {
		specs, typ, fieldStruct, _ := ast.SignatureGCDeclaration(prgrm, prgrm.GetCXTypeSignatureFromArray(fieldIdx))
		if gcValueReferences(prgrm, specs, typ, fieldStruct, depth+1) {
			return true
		}
	}
	return false
}
