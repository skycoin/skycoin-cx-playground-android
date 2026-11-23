package util

import (
	"errors"
	"io/fs"
)

// FileErrorCode is a file failure code, 1001 to 1499, see docs/ErrorHandling.md.
type FileErrorCode uint32

// FileOperationCode is a file operation code, 1500 to 1999, see docs/ErrorHandling.md.
type FileOperationCode uint32

const (
	FileErrorCodeNotFound              FileErrorCode = 1001
	FileErrorCodePermissionDenied      FileErrorCode = 1002
	FileErrorCodePathInvalid           FileErrorCode = 1003
	FileErrorCodePathOutsideSandbox    FileErrorCode = 1004
	FileErrorCodeSandboxRootMissing    FileErrorCode = 1005
	FileErrorCodeFileSystemCallFailure FileErrorCode = 1006
)

const (
	FileOperationOpen            FileOperationCode = 1501
	FileOperationCreate          FileOperationCode = 1502
	FileOperationRead            FileOperationCode = 1503
	FileOperationPathValidate    FileOperationCode = 1504
	FileOperationSandboxRootOpen FileOperationCode = 1505
)

// FileError is a class A failure of a file operation, nil on success.
type FileError struct {
	FileErrorCode      FileErrorCode
	FileErrorOperation FileOperationCode
	FileErrorMessage   string
	FileErrorPath      string
}

// fileSystemPathEscapeMessage is the text of os.errPathEscapes, which os.Root
// returns for a path or symlink that leaves the root (go/src/os/file.go).
const fileSystemPathEscapeMessage = "path escapes from parent"

// fileErrorFromFileSystemError wraps a Go error from the os package.
func fileErrorFromFileSystemError(operation FileOperationCode, path string, fileSystemError error) *FileError {
	code := FileErrorCodeFileSystemCallFailure
	var pathError *fs.PathError
	switch {
	case errors.As(fileSystemError, &pathError) && pathError.Err.Error() == fileSystemPathEscapeMessage:
		code = FileErrorCodePathOutsideSandbox
	case errors.Is(fileSystemError, fs.ErrNotExist):
		code = FileErrorCodeNotFound
	case errors.Is(fileSystemError, fs.ErrPermission):
		code = FileErrorCodePermissionDenied
	}
	return &FileError{
		FileErrorCode:      code,
		FileErrorOperation: operation,
		FileErrorMessage:   fileSystemError.Error(),
		FileErrorPath:      path,
	}
}
