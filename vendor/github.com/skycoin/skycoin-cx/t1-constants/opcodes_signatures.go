package constants

import (
	"github.com/skycoin/skycoin-cx/t0-types" // thin: only Code; core types, keep
)

// OpcodeGroup is the package that holds the handler of an opcode. It decides
// the build tags the opcode is linked under, see OpcodeLinked.
type OpcodeGroup uint8

// Opcode groups. OpcodeGroupNone is a number without a definition row.
const (
	OpcodeGroupNone OpcodeGroup = iota
	// OpcodeGroupCore is t4-opcodes and t4-cx-runtime-printer, and the rows
	// without a handler.
	OpcodeGroupCore
	// OpcodeGroupOsCore is t4-cxos-core, left out by the nocxos tag.
	OpcodeGroupOsCore
	// OpcodeGroupRegexp is t4-regexp, left out by the noregexp tag.
	OpcodeGroupRegexp
	// OpcodeGroupCxfx is t6-cxfx, linked only with the cxfx tag.
	OpcodeGroupCxfx
	OpcodeGroupCount
)

// OpcodeParameterForm is the kind of an opcode parameter.
type OpcodeParameterForm uint8

// Opcode parameter forms.
const (
	OpcodeParameterFormNone OpcodeParameterForm = iota
	OpcodeParameterFormAtomic
	OpcodeParameterFormSlice
	OpcodeParameterFormStruct
)

// OpcodeParameter is one input or output of an opcode signature. Type is the
// atomic type or the element type of a slice. Struct is the row of
// OpcodeParameterStructTable of a struct parameter.
type OpcodeParameter struct {
	OpcodeParameterForm   OpcodeParameterForm
	OpcodeParameterType   types.Code
	OpcodeParameterStruct uint8
}

// OpcodeParameterStruct is a struct type an opcode takes or returns, with the
// name the parameter has.
type OpcodeParameterStruct struct {
	OpcodeParameterStructPackage string
	OpcodeParameterStructType    string
	OpcodeParameterStructName    string
}

// OpcodeSignature is the signature of one opcode. Its inputs and then its
// outputs are the rows of OpcodeParameterTable from ParameterFirst on.
type OpcodeSignature struct {
	OpcodeSignatureGroup          OpcodeGroup
	OpcodeSignatureHandled        bool
	OpcodeSignatureParameterFirst uint16
	OpcodeSignatureInputCount     uint8
	OpcodeSignatureOutputCount    uint8
}

// OpcodeSignatureOf returns the signature of opcode number code, with
// OpcodeGroupNone when it has no definition row.
func OpcodeSignatureOf(code int) OpcodeSignature {
	if code <= 0 || code >= len(OpcodeSignatureTable) {
		return OpcodeSignature{}
	}
	return OpcodeSignatureTable[code]
}

// OpcodeSignatureInputs returns the input parameters of signature. The caller
// must not change them.
func (signature OpcodeSignature) OpcodeSignatureInputs() []OpcodeParameter {
	first := int(signature.OpcodeSignatureParameterFirst)
	return OpcodeParameterTable[first : first+int(signature.OpcodeSignatureInputCount)]
}

// OpcodeSignatureOutputs returns the output parameters of signature. The
// caller must not change them.
func (signature OpcodeSignature) OpcodeSignatureOutputs() []OpcodeParameter {
	first := int(signature.OpcodeSignatureParameterFirst) + int(signature.OpcodeSignatureInputCount)
	return OpcodeParameterTable[first : first+int(signature.OpcodeSignatureOutputCount)]
}

// OpcodeOperatorOf returns the number of the typed form of the untyped
// operator for operands of type typeCode, 0 when there is none.
func OpcodeOperatorOf(typeCode types.Code, operator int) int {
	if typeCode >= types.COUNT || operator <= START_OF_OPERATORS || operator >= END_OF_OPERATORS {
		return 0
	}
	return int(OpcodeOperatorTable[typeCode][operator-START_OF_OPERATORS-1])
}

// OpcodeNameOf returns the name of the live opcode code, "" when none is.
func OpcodeNameOf(code int) string {
	if code <= 0 || code >= len(OpcodeNameRowTable) || OpcodeNameRowTable[code] == 0 {
		return ""
	}
	return OpcodeLifecycleTable[OpcodeNameRowTable[code]-1].Name
}

// OpcodeNameBytesOf returns the bytes of OpcodeNameOf without building a
// string, nil when no name is. The caller must not write to them.
func OpcodeNameBytesOf(code int) []byte {
	if code <= 0 || code >= len(OpcodeNameRowTable) || OpcodeNameRowTable[code] == 0 {
		return nil
	}
	return opcodeLifecycleNames[OpcodeNameRowTable[code]-1]
}

// opcodeLifecycleNames holds the Name of each row of OpcodeLifecycleTable as
// bytes, built while the package initializes.
var opcodeLifecycleNames = opcodeLifecycleNamesMake()

func opcodeLifecycleNamesMake() [][]byte {
	names := make([][]byte, len(OpcodeLifecycleTable))
	for i := range OpcodeLifecycleTable {
		names[i] = []byte(OpcodeLifecycleTable[i].Name)
		names[i] = names[i][:len(names[i]):len(names[i])]
	}
	return names
}

// NativeName is one row of NativeNameTable, the name a program calls a
// built-in by and its code, an opcode number or a LibraryFunctionKey.
type NativeName struct {
	NativeNameText string
	NativeNameCode int
}

// libraryFunctionKeyShift leaves room for 65535 functions per library.
const libraryFunctionKeyShift = 16

// LibraryFunctionKey returns the code of a library function in
// NativeNameTable. It is negative, so it never meets an opcode number.
func LibraryFunctionKey(library int, function int) int {
	return -(library<<libraryFunctionKeyShift | function)
}

// LibraryFunctionKeyParts returns the library and function numbers of key.
func LibraryFunctionKeyParts(key int) (library int, function int) {
	return -key >> libraryFunctionKeyShift, -key & (1<<libraryFunctionKeyShift - 1)
}

// opcodeGroupLinked says which groups this build links, by its build tags.
var opcodeGroupLinked = [OpcodeGroupCount]bool{
	OpcodeGroupCore:   true,
	OpcodeGroupOsCore: opcodeGroupOsCoreLinked,
	OpcodeGroupRegexp: opcodeGroupRegexpLinked,
	OpcodeGroupCxfx:   opcodeGroupCxfxLinked,
}

// OpcodeGroupLinked reports whether this build links group.
func OpcodeGroupLinked(group OpcodeGroup) bool {
	return opcodeGroupLinked[group]
}

// OpcodeLinked reports whether opcode code has a definition row and this
// build links its group, so the compiler knows its name.
func OpcodeLinked(code int) bool {
	return opcodeGroupLinked[OpcodeSignatureOf(code).OpcodeSignatureGroup]
}

// OpcodeHandled reports whether opcode code has a handler the opcode switch
// calls. The compiler replaces the opcodes without one.
func OpcodeHandled(code int) bool {
	return OpcodeSignatureOf(code).OpcodeSignatureHandled
}
