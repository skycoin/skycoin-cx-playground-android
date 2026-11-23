package ast

import (
	"os"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t0-util" // thin: only the program path and sandbox calls; no depth gain
)

// ProgramStringBytesRead returns the bytes of the str in value. They alias
// program memory, so the caller copies them before the program runs on.
func ProgramStringBytesRead(prgrm *CXProgram, value *CXValue) []byte {
	return types.Read_obj(prgrm.Memory, value.Offset)
}

// ProgramFileSystemPathRead validates the str in value as a program path,
// reading its bytes from program memory without a Go string, see #433.
func ProgramFileSystemPathRead(prgrm *CXProgram, value *CXValue) (util.ProgramFileSystemPath, *util.FileError) {
	return util.ProgramFileSystemPathFromBytes(ProgramStringBytesRead(prgrm, value))
}

// ProgramSandboxFileOpen opens the program path in value for reading inside
// the program's sandbox root.
func ProgramSandboxFileOpen(prgrm *CXProgram, value *CXValue) (*os.File, *util.FileError) {
	programPath, fileError := ProgramFileSystemPathRead(prgrm, value)
	if fileError != nil {
		return nil, fileError
	}
	return util.ProgramSandboxFileOpen(prgrm.ProgramSandboxRoot, programPath)
}

// ProgramSandboxFileCreate creates the program path in value inside the
// program's sandbox root.
func ProgramSandboxFileCreate(prgrm *CXProgram, value *CXValue) (*os.File, *util.FileError) {
	programPath, fileError := ProgramFileSystemPathRead(prgrm, value)
	if fileError != nil {
		return nil, fileError
	}
	return util.ProgramSandboxFileCreate(prgrm.ProgramSandboxRoot, programPath)
}

// ProgramSandboxFileRead reads the whole program path in value inside the
// program's sandbox root.
func ProgramSandboxFileRead(prgrm *CXProgram, value *CXValue) ([]byte, *util.FileError) {
	programPath, fileError := ProgramFileSystemPathRead(prgrm, value)
	if fileError != nil {
		return nil, fileError
	}
	return util.ProgramSandboxFileRead(prgrm.ProgramSandboxRoot, programPath)
}
