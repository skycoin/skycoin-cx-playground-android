//go:build !cxopcount

package ast

// operationCountEnabled is a constant, so the normal build compiles the
// count out of CXCall.Call.
const operationCountEnabled = false
