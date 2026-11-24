//go:build !nocxos

package cxos

import (
	"fmt"
	"os"

	"github.com/skycoin/skycoin-cx/t0-util"
	"github.com/skycoin/skycoin-cx/t1-profiling" // thin: only StartCPUProfileOnFile; no depth gain
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// VIOLATION: state outside State. Owner: t5-cxos; moves to OsFileMng in phase 1.
var openProfiles = make(map[string]func() error)

// opStartProfile writes a CPU profile to a program path in the sandbox.
// VIOLATION STRHOT-139: B, hands a path or text to Go or the OS, #412.
// VIOLATION VMALLOC-018: a system library call allocates in Go and the OS, #407.
func opStartProfile(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	programPath, fileError := ast.ProgramFileSystemPathRead(prgrm, &inputs[0])
	if fileError != nil {
		fmt.Fprintln(os.Stderr, fileError.FileErrorMessage)
		return
	}
	profRate := int(inputs[1].Get_i32(prgrm))
	if profRate == 0 {
		return
	}
	file, fileError := util.ProgramSandboxFileCreate(prgrm.ProgramSandboxRoot, programPath)
	if fileError != nil {
		fmt.Fprintln(os.Stderr, fileError.FileErrorMessage)
		return
	}
	stop, err := profiling.StartCPUProfileOnFile(file, profRate)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	openProfiles[programPath.ProgramFileSystemPathText()] = stop
}

// VIOLATION STRHOT-140: B, hands a path or text to Go or the OS, #412.
func opStopProfile(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	programPath, fileError := ast.ProgramFileSystemPathRead(prgrm, &inputs[0])
	if fileError != nil {
		return
	}
	stop, ok := openProfiles[programPath.ProgramFileSystemPathText()]
	if !ok {
		return
	}
	delete(openProfiles, programPath.ProgramFileSystemPathText())
	if err := stop(); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
