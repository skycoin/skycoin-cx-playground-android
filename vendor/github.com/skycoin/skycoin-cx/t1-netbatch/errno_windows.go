//go:build windows

package netbatch

import "syscall"

// The Winsock error numbers netErrorOf reads, see
// https://learn.microsoft.com/en-us/windows/win32/winsock/windows-sockets-error-codes-2
var (
	netErrnoRefused      error = syscall.Errno(10061)
	netErrnoReset        error = syscall.Errno(10054)
	netErrnoBrokenPipe   error = syscall.Errno(10053)
	netErrnoAddressInUse error = syscall.Errno(10048)
)
