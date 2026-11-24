//go:build !nocxos

package cxos

import (
	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only In, Out, RegisterFunction; registration API, keep
)

// RegisterPackage registers the file and process libraries.
func RegisterPackage(prgrm *ast.CXProgram) {
	LibraryFileRegister(prgrm)
	LibraryProcessRegister(prgrm)
}

// LibraryFileRegister registers the functions of the file library, the file
// functions of sys/os, sys/json and the CPU profile.
func LibraryFileRegister(prgrm *ast.CXProgram) {
	// os
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.GetWorkingDirectory", opOsGetWorkingDirectory, nil, opcodes.Out(ast.ConstCxArg_STR))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.LogFile", opOsLogFile, opcodes.In(ast.ConstCxArg_BOOL), nil)
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.Open", opOsOpen, opcodes.In(ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_I32))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.Create", opOsCreate, opcodes.In(ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_I32))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.Close", opOsClose, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.Seek", opOsSeek, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_I64, ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_I64))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadStr", opOsReadStr, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_STR, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadF64", opOsReadF64, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_F64, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadF32", opOsReadF32, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_F32, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadUI64", opOsReadUI64, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_UI64, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadUI32", opOsReadUI32, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_UI32, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadUI16", opOsReadUI16, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_UI16, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadUI8", opOsReadUI8, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_UI8, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadI64", opOsReadI64, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_I64, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadI32", opOsReadI32, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_I32, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadI16", opOsReadI16, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_I16, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadI8", opOsReadI8, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCXArg_I8, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadBOOL", opOsReadBOOL, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_BOOL, ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1368.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadF64Slice", opOsReadF64Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.F64), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.F64), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1369.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadF32Slice", opOsReadF32Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.F32), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.F32), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1370.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadUI64Slice", opOsReadUI64Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI64), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.UI64), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1371.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadUI32Slice", opOsReadUI32Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI32), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.UI32), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1372.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadUI16Slice", opOsReadUI16Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI16), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.UI16), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1373.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadUI8Slice", opOsReadUI8Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI8), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.UI8), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1374.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadI64Slice", opOsReadI64Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.I64), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.I64), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1375.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadI32Slice", opOsReadI32Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.I32), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.I32), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1376.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadI16Slice", opOsReadI16Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.I16), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.I16), ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1377.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadI8Slice", opOsReadI8Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.I8), ast.ConstCxArg_UI64), opcodes.Out(ast.Slice(types.I8), ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.ReadAllText", opOsReadAllText, opcodes.In(ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_STR, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteStr", opOsWriteStr, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteF64", opOsWriteF64, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_F64), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteF32", opOsWriteF32, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_F32), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteUI64", opOsWriteUI64, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_UI64), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteUI32", opOsWriteUI32, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_UI32), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteUI16", opOsWriteUI16, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_UI16), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteUI8", opOsWriteUI8, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_UI8), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteI64", opOsWriteI64, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_I64), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteI32", opOsWriteI32, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteI16", opOsWriteI16, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_I16), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteI8", opOsWriteI8, opcodes.In(ast.ConstCxArg_I32, ast.ConstCXArg_I8), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteBOOL", opOsWriteBOOL, opcodes.In(ast.ConstCxArg_I32, ast.ConstCxArg_BOOL), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1378.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteF64Slice", opOsWriteF64Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.F64)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1379.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteF32Slice", opOsWriteF32Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.F32)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1380.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteUI64Slice", opOsWriteUI64Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI64)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1381.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteUI32Slice", opOsWriteUI32Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI32)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1382.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteUI16Slice", opOsWriteUI16Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI16)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1383.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteUI8Slice", opOsWriteUI8Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.UI8)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1384.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteI64Slice", opOsWriteI64Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.I64)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1385.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteI32Slice", opOsWriteI32Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.I32)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1386.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteI16Slice", opOsWriteI16Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.I16)), opcodes.Out(ast.ConstCxArg_BOOL))
	// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
	// Now a constructor call, site S1387.
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "os.WriteI8Slice", opOsWriteI8Slice, opcodes.In(ast.ConstCxArg_I32, ast.Slice(types.I8)), opcodes.Out(ast.ConstCxArg_BOOL))

	opcodes.RegisterLibraryFunctionVm(constants.LibraryNumberFile, "os.Close", libraryFileCloseVm)
	opcodes.RegisterLibraryFunctionVm(constants.LibraryNumberFile, "os.Seek", libraryFileSeekVm)

	// json
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Open", opJsonOpen, opcodes.In(ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_I32))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Close", opJsonClose, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.More", opJsonTokenMore, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_BOOL, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Next", opJsonTokenNext, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_I32, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Type", opJsonTokenType, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_I32, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Delim", opJsonTokenDelim, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_I32, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Bool", opJsonTokenBool, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_BOOL, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Float64", opJsonTokenF64, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_F64, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Int64", opJsonTokenI64, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_I64, ast.ConstCxArg_BOOL))
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "json.Str", opJsonTokenStr, opcodes.In(ast.ConstCxArg_I32), opcodes.Out(ast.ConstCxArg_STR, ast.ConstCxArg_BOOL))

	// profile
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "StartCPUProfile", opStartProfile, opcodes.In(ast.ConstCxArg_STR, ast.ConstCxArg_I32), nil)
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberFile, "StopCPUProfile", opStopProfile, opcodes.In(ast.ConstCxArg_STR), nil)
}

// LibraryProcessRegister registers the process library, os.Run.
func LibraryProcessRegister(prgrm *ast.CXProgram) {
	opcodes.RegisterLibraryFunction(prgrm, constants.LibraryNumberProcess, "os.Run", opOsRun, opcodes.In(ast.ConstCxArg_STR, ast.ConstCxArg_I32, ast.ConstCxArg_I32, ast.ConstCxArg_STR), opcodes.Out(ast.ConstCxArg_I32, ast.ConstCxArg_I32, ast.ConstCxArg_STR))
}
