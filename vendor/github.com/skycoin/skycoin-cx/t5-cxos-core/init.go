//go:build !nocxos

// Package cxoscore holds the time and os opcodes that need no file system,
// so that the core binary of cx build can include them without t5-cxos.
package cxoscore

import (
	"github.com/skycoin/skycoin-cx/t3-ast"
	"github.com/skycoin/skycoin-cx/t4-opcodes" // thin: only In, Out, RegisterFunction; registration API, keep
)

// RegisterPackage adds the time opcodes and os.Exit to the opcode tables.
func RegisterPackage(prgrm *ast.CXProgram) {
	opcodes.RegisterFunction(prgrm, "time.Sleep", opTimeSleep, opcodes.In(ast.ConstCxArg_I32), nil)
	opcodes.RegisterFunction(prgrm, "time.UnixMilli", opTimeUnixMilli, nil, opcodes.Out(ast.ConstCxArg_I64))
	opcodes.RegisterFunction(prgrm, "time.UnixNano", opTimeUnixNano, nil, opcodes.Out(ast.ConstCxArg_I64))
	opcodes.RegisterFunction(prgrm, "os.Exit", opOsExit, opcodes.In(ast.ConstCxArg_I32), nil)
}
