package util

import "unsafe"

// ProgramTextStringView returns text as a Go string that shares its bytes, for
// a Go API that takes a string and does not keep it. Only t6-cxfx calls it, see #432.
// VIOLATION STRHOT-322: B, a string view of program bytes for the GLFW, gltext and file APIs, no copy, #412.
func ProgramTextStringView(text []byte) string {
	return unsafe.String(unsafe.SliceData(text), len(text))
}
