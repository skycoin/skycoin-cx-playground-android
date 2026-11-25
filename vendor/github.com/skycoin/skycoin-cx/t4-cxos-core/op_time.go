//go:build !nocxos

package cxoscore

import (
	"time"

	"github.com/skycoin/skycoin-cx/t3-ast"
)

func makeTimestamp() int64 {
	return time.Now().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
}

func OpTimeUnixMilli(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i64(prgrm, makeTimestamp())
}

func OpTimeUnixNano(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	outputs[0].Set_i64(prgrm, time.Now().UnixNano())
}

func OpTimeSleep(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	time.Sleep(time.Duration(inputs[0].Get_i32(prgrm)) * time.Millisecond)
}
