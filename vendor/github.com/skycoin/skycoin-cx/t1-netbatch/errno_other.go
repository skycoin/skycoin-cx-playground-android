//go:build !unix && !windows

package netbatch

// js/wasm and wasip1 have no error numbers to read, and errors.Is never
// matches a nil target.
var netErrnoRefused, netErrnoReset, netErrnoBrokenPipe, netErrnoAddressInUse error
