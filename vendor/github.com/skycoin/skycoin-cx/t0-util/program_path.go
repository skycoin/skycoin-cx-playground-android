package util

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
)

// ProgramFileSystemPathLengthMaximum is the longest path a program may pass, in bytes.
const ProgramFileSystemPathLengthMaximum = 4096

// ProgramFileSystemPath is a path a running CX program passes to a file
// opcode, validated to stay inside the sandbox root, see #414 and #433.
type ProgramFileSystemPath struct {
	programFileSystemPathText string
}

// ProgramFileSystemPathText returns the cleaned, slash separated path.
func (programPath ProgramFileSystemPath) ProgramFileSystemPathText() string {
	return programPath.programFileSystemPathText
}

// ProgramFileSystemPathFromBytes validates a path read from program memory.
// It must be relative, use only '/' and not start with ".." once cleaned.
func ProgramFileSystemPathFromBytes(pathBytes []byte) (ProgramFileSystemPath, *FileError) {
	invalid := func(code FileErrorCode, reason string) (ProgramFileSystemPath, *FileError) {
		fileError := &FileError{
			FileErrorCode:      code,
			FileErrorOperation: FileOperationPathValidate,
			FileErrorMessage:   fmt.Sprintf("program path %q: %s", pathBytes, reason),
			FileErrorPath:      string(pathBytes),
		}
		if logFile {
			fmt.Printf("program path rejected: %s\n", fileError.FileErrorMessage)
		}
		return ProgramFileSystemPath{}, fileError
	}
	switch {
	case len(pathBytes) == 0:
		return invalid(FileErrorCodePathInvalid, "empty")
	case len(pathBytes) > ProgramFileSystemPathLengthMaximum:
		return invalid(FileErrorCodePathInvalid, "longer than 4096 bytes")
	case bytes.IndexByte(pathBytes, 0) >= 0:
		return invalid(FileErrorCodePathInvalid, "contains a NUL byte")
	case bytes.IndexByte(pathBytes, '\\') >= 0:
		return invalid(FileErrorCodePathInvalid, "contains '\\', use '/'")
	case pathBytes[0] == '/':
		return invalid(FileErrorCodePathOutsideSandbox, "absolute")
	case len(pathBytes) >= 2 && pathBytes[1] == ':' && programPathDriveLetter(pathBytes[0]):
		return invalid(FileErrorCodePathOutsideSandbox, "has a volume name")
	}
	cleaned := path.Clean(string(pathBytes))
	if cleaned == ".." || len(cleaned) >= 3 && cleaned[:3] == "../" {
		return invalid(FileErrorCodePathOutsideSandbox, "leaves the sandbox")
	}
	return ProgramFileSystemPath{programFileSystemPathText: cleaned}, nil
}

func programPathDriveLetter(letter byte) bool {
	return letter >= 'A' && letter <= 'Z' || letter >= 'a' && letter <= 'z'
}

// ProgramSandboxRootOpen opens the directory every program path resolves in.
// An empty dir means the working directory set by CXSetWorkingDir, else ".".
func ProgramSandboxRootOpen(dir string) (*os.Root, *FileError) {
	if dir == "" {
		dir = workingDir
	}
	if dir == "" {
		dir = "."
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fileErrorFromFileSystemError(FileOperationSandboxRootOpen, dir, err)
	}
	return root, nil
}

func programSandboxRootCheck(root *os.Root, operation FileOperationCode, programPath ProgramFileSystemPath) *FileError {
	if root != nil {
		return nil
	}
	return &FileError{
		FileErrorCode:      FileErrorCodeSandboxRootMissing,
		FileErrorOperation: operation,
		FileErrorMessage:   "no sandbox root is open for this program",
		FileErrorPath:      programPath.programFileSystemPathText,
	}
}

// ProgramSandboxFileOpen opens a program path for reading inside root.
func ProgramSandboxFileOpen(root *os.Root, programPath ProgramFileSystemPath) (*os.File, *FileError) {
	if fileError := programSandboxRootCheck(root, FileOperationOpen, programPath); fileError != nil {
		return nil, fileError
	}
	if logFile {
		fmt.Printf("ProgramSandboxFileOpen: Opening '%s' in '%s'\n", programPath.programFileSystemPathText, root.Name())
	}
	file, err := root.Open(programPath.programFileSystemPathText)
	if err != nil {
		return nil, programSandboxFileError(FileOperationOpen, programPath, err)
	}
	return file, nil
}

// ProgramSandboxFileCreate creates or truncates a program path inside root.
func ProgramSandboxFileCreate(root *os.Root, programPath ProgramFileSystemPath) (*os.File, *FileError) {
	if fileError := programSandboxRootCheck(root, FileOperationCreate, programPath); fileError != nil {
		return nil, fileError
	}
	if logFile {
		fmt.Printf("ProgramSandboxFileCreate: Creating '%s' in '%s'\n", programPath.programFileSystemPathText, root.Name())
	}
	file, err := root.Create(programPath.programFileSystemPathText)
	if err != nil {
		return nil, programSandboxFileError(FileOperationCreate, programPath, err)
	}
	return file, nil
}

// ProgramSandboxFileRead reads a whole program path inside root.
func ProgramSandboxFileRead(root *os.Root, programPath ProgramFileSystemPath) ([]byte, *FileError) {
	file, fileError := ProgramSandboxFileOpen(root, programPath)
	if fileError != nil {
		fileError.FileErrorOperation = FileOperationRead
		return nil, fileError
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, programSandboxFileError(FileOperationRead, programPath, err)
	}
	return content, nil
}

func programSandboxFileError(operation FileOperationCode, programPath ProgramFileSystemPath, err error) *FileError {
	fileError := fileErrorFromFileSystemError(operation, programPath.programFileSystemPathText, err)
	if logFile {
		fmt.Printf("program file operation %d failed: %s\n", operation, fileError.FileErrorMessage)
	}
	return fileError
}
