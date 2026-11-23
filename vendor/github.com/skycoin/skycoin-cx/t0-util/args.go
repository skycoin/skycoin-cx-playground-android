package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

// ParseArgsForCX splits args into "++" cx flags and .cx source files. A
// directory argument adds every .cx file in it, and in its subdirectories
// when alsoSubdirs is set. On error every file it opened is closed again.
func ParseArgsForCX(args []string, alsoSubdirs bool) (cxArgs []string, sourceCode []*os.File, fileNames []string, err error) {
	defer func() {
		if err != nil {
			for _, file := range sourceCode {
				file.Close()
			}
			cxArgs, sourceCode, fileNames = nil, nil, nil
		}
	}()

	skip := false // flag for skipping arg

	for _, arg := range args {

		// skip arg if skip flag is specified
		if skip {
			skip = false
			continue
		}

		// cli flags are either "--key=value" or "-key value"
		// we have to skip both cases
		// NOTE: assumes every flag without = takes a value, so a bool flag swallows the
		// next argument, which may be a source file. See #101.
		if len(arg) > 1 && arg[0] == '-' {
			if !strings.Contains(arg, "=") {
				skip = true
			}
			continue
		}

		// cli cx flags are prefixed with "++"
		if len(arg) > 2 && arg[:2] == "++" {
			cxArgs = append(cxArgs, arg)
			continue
		}

		arg = filepath.Clean(arg)
		// NOTE: CXStatFile, the directory walk and CXOpenFile apply the working directory
		// in different ways. See #100.
		fi, err := CXStatFile(arg)
		if err != nil {
			return cxArgs, sourceCode, fileNames, fmt.Errorf("%s: source file or library not found: %w", arg, err)
		}

		switch mode := fi.Mode(); {
		case mode.IsDir():
			var fileList []string

			// Checking if we want to check all subdirectories.
			if alsoSubdirs {
				fileList, err = listDirRecursive(arg)
			} else {
				fileList, err = listDir(arg)
			}
			if err != nil {
				return cxArgs, sourceCode, fileNames, fmt.Errorf("%s: list source files: %w", arg, err)
			}

			for _, path := range fileList {
				// only loading .cx files
				if filepath.Ext(path) != ".cx" {
					continue
				}
				file, err := CXOpenFile(path)
				if err != nil {
					return cxArgs, sourceCode, fileNames, fmt.Errorf("%s: open source file: %w", path, err)
				}
				sourceCode = append(sourceCode, file)
				fileNames = append(fileNames, file.Name())
			}
		case mode.IsRegular():
			file, err := CXOpenFile(arg)
			if err != nil {
				return cxArgs, sourceCode, fileNames, fmt.Errorf("%s: open source file: %w", arg, err)
			}
			fileNames = append(fileNames, file.Name())
			sourceCode = append(sourceCode, file)
		default:
			return cxArgs, sourceCode, fileNames, fmt.Errorf("%s: not a regular file or directory", arg)
		}
	}

	return cxArgs, sourceCode, fileNames, nil
}

func listDirRecursive(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func listDir(root string) ([]string, error) {
	var files []string
	fileInfo, err := ioutil.ReadDir(root)
	if err != nil {
		return files, err
	}

	for _, file := range fileInfo {
		if file.IsDir() {
			continue
		}
		files = append(files, filepath.Join(root, file.Name()))
	}
	return files, nil
}
