//go:build !cxopcount

package vm

// operationCountEnabled is a constant, so the normal build compiles the
// count out of the loop.
const operationCountEnabled = false
