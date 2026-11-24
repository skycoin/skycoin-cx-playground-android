package ast

import stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"

// CXProgramNameIntern interns a name in the string pool of the program, see
// #413. It is called while compiling only.
func (prgrm *CXProgram) CXProgramNameIntern(text string) stringpool.StringGid {
	return prgrm.CxIrState.StringPool.StringPoolIntern([]byte(text))
}

// CXProgramNameFind returns the Gid of a name without interning it, and false
// when no name has that text. It works on a frozen pool, for run time lookups.
// VIOLATION STRHOT-283: B, name text for compiler code run by an opcode, #412.
func (prgrm *CXProgram) CXProgramNameFind(text string) (stringpool.StringGid, bool) {
	return prgrm.CxIrState.StringPool.StringPoolFind([]byte(text))
}

// CXProgramNameFindBytes is CXProgramNameFind for a name read from program
// memory, so the caller needs no Go string.
func (prgrm *CXProgram) CXProgramNameFindBytes(text []byte) (stringpool.StringGid, bool) {
	return prgrm.CxIrState.StringPool.StringPoolFind(text)
}

// CXProgramNameBytes returns the bytes of a name. Only boundary code calls it,
// diagnostics, printing, the serializer and the REPL. The caller must not
// write to them.
func (prgrm *CXProgram) CXProgramNameBytes(gid stringpool.StringGid) []byte {
	return prgrm.CxIrState.StringPool.StringPoolBytes(gid)
}

// CXProgramNameText returns a name as a Go string that views the pool, for
// boundary code that still takes strings. It does not allocate.
// VIOLATION STRHOT-284: B, name text for compiler code run by an opcode, #412.
func (prgrm *CXProgram) CXProgramNameText(gid stringpool.StringGid) string {
	return prgrm.CxIrState.StringPool.StringPoolText(gid)
}

// CXProgramMethodNameIntern interns "receiverName.methodName", the function
// name of a method. It is called while compiling only.
func (prgrm *CXProgram) CXProgramMethodNameIntern(receiverName stringpool.StringGid, methodName stringpool.StringGid) stringpool.StringGid {
	receiver := prgrm.CXProgramNameBytes(receiverName)
	method := prgrm.CXProgramNameBytes(methodName)
	name := make([]byte, 0, len(receiver)+1+len(method))
	name = append(name, receiver...)
	name = append(name, '.')
	name = append(name, method...)
	return prgrm.CxIrState.StringPool.StringPoolIntern(name)
}

// CXProgramSourceFileIntern returns the Gid of the source file at path,
// registering it on first use. It is called while compiling only.
func (prgrm *CXProgram) CXProgramSourceFileIntern(path string) SourceFileGid {
	return prgrm.CxIrState.SourceFileMng.SourceFileIntern(prgrm.CxIrState.StringPool.StringPoolIntern([]byte(path)))
}

// CXProgramSourceFileText returns the path of a source file as a Go string
// that views the pool, for diagnostics, printing and the serializer.
// VIOLATION STRHOT-285: E, error or assert text, built on a failure branch only, #412.
func (prgrm *CXProgram) CXProgramSourceFileText(gid SourceFileGid) string {
	return prgrm.CxIrState.StringPool.StringPoolText(prgrm.CxIrState.SourceFileMng.SourceFilePath(gid))
}
