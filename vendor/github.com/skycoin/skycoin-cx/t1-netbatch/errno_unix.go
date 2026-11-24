//go:build unix

package netbatch

import "syscall"

// The errno values netErrorOf reads.
var (
	netErrnoRefused      error = syscall.ECONNREFUSED
	netErrnoReset        error = syscall.ECONNRESET
	netErrnoBrokenPipe   error = syscall.EPIPE
	netErrnoAddressInUse error = syscall.EADDRINUSE
)
