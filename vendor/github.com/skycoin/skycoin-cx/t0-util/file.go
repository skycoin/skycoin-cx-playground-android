package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

// VIOLATION: state outside State. Owner: t0-util; moves to ConfigMng in phase 2.
var workingDir string

// VIOLATION: state outside State. Owner: t0-util; moves to ConfigMng in phase 2.
var logFile bool

// CXSetWorkingDir ...
func CXSetWorkingDir(dir string) {
	workingDir = dir
}

// CXLogFile ...
func CXLogFile(enable bool) {
	logFile = enable
}

// CXOpenFile ...
func CXOpenFile(filename string) (*os.File, error) {
	filename = cxPath(filename)

	if logFile {
		fmt.Printf("CXOpenFile: Opening '%s'\n", filename)
	}

	file, err := os.Open(filename)
	if logFile && err != nil {
		fmt.Printf("CXOpenFile: Failed to open '%s': %v\n", filename, err)
	}
	return file, err
}

// CXCreateFile ...
func CXCreateFile(filename string) (*os.File, error) {
	filename = cxPath(filename)

	if logFile {
		fmt.Printf("Creating file : '%s', '%s'\n", workingDir, filename)
	}

	file, err := os.Create(filename)
	if logFile && err != nil {
		fmt.Printf("Failed to create file : '%s', '%s', err '%v'\n", workingDir, filename, err)
	}

	return file, err
}

// CXRemoveFile ...
func CXRemoveFile(path string) error {
	if logFile {
		fmt.Printf("Removing file : '%s', '%s'\n", workingDir, path)
	}

	err := os.Remove(cxPath(path))

	if logFile && err != nil {
		fmt.Printf("Failed to remove file : '%s', '%s', err '%v'\n", workingDir, path, err)
	}

	return err
}

// CXReadFile ...
func CXReadFile(path string) ([]byte, error) {
	if logFile {
		fmt.Printf("Reading file : '%s', '%s'\n", workingDir, path)
	}

	bytes, err := ioutil.ReadFile(cxPath(path))

	if logFile && err != nil {
		fmt.Printf("Failed to read file : '%s', '%s', err '%v'\n", workingDir, path, err)
	}

	return bytes, err
}

// CXStatFile ...
func CXStatFile(path string) (os.FileInfo, error) {
	if logFile {
		fmt.Printf("Stating file : '%s', '%s'\n", workingDir, path)
	}

	fileInfo, err := os.Stat(cxPath(path))

	if logFile && err != nil {
		fmt.Printf("Failed to stat file : '%s', '%s', err '%v'\n", workingDir, path, err)
	}

	return fileInfo, err
}

// CXMkdirAll ...
func CXMkdirAll(path string, perm os.FileMode) error {
	if logFile {
		fmt.Printf("Creating dir : '%s'\n", path)
	}

	err := os.MkdirAll(cxPath(path), perm)

	if logFile && err != nil {
		fmt.Printf("Failed to create dir : '%s', '%s', err '%v'\n", workingDir, path, err)
	}

	return err
}

// cxPath returns name relative to the working dir set by CXSetWorkingDir.
func cxPath(name string) string {
	return filepath.Join(workingDir, name)
}
