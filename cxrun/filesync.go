package cxrun

import (
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	cxos "github.com/skycoin/skycoin-cx/t5-cxos"
)

// opFileSync is app.FileSync(file i32) (ok bool): it flushes a file that
// os.Open or os.Create returned to the storage device. skycoin-cx's os
// package has no sync, and the direct block storage test of decision D29
// syncs after every write. It is false for a handle that is not open.
func opFileSync(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	file := cxos.ValidFile(in[0].Get_i32(p))
	out[0].Set_bool(p, file != nil && file.Sync() == nil)
}
