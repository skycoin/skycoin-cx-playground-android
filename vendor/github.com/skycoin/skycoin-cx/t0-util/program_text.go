package util

import "unsafe"

// ProgramTextStringView returns text as a Go string that shares its bytes, for
// a Go API that takes a string and does not keep it, see #432.
// VIOLATION STRHOT-322: O, a string view of program bytes for ParseFloat, no copy, #412.
func ProgramTextStringView(text []byte) string {
	return unsafe.String(unsafe.SliceData(text), len(text))
}
