package opcodes

import (
	"os"

	// GCREACH: t3-ast owns the heap and the GC roots. This file formats into a Go
	// buffer and allocates nothing, see docs/GarbageCollector.md.
	"github.com/skycoin/skycoin-cx/t3-ast"
)

// formatBuffer returns the empty scratch buffer of prgrm that the print,
// printf and str conversion opcodes append to (#412).
func formatBuffer(prgrm *ast.CXProgram) []byte {
	return prgrm.ProgramFormatBuffer[:0]
}

// formatBufferKeep keeps formatted, which appending may have grown, as the
// scratch buffer of the next opcode.
func formatBufferKeep(prgrm *ast.CXProgram, formatted []byte) {
	prgrm.ProgramFormatBuffer = formatted[:0]
}

// printFormattedLine writes formatted and a new line to standard output in one
// write, as fmt.Println did.
// VIOLATION VMALLOC-022: appending grows the format buffer past its size, once, #407.
func printFormattedLine(prgrm *ast.CXProgram, formatted []byte) {
	line := append(formatted, '\n')
	os.Stdout.Write(line)
	formatBufferKeep(prgrm, line)
}

// parseDecimalUint parses text as strconv.ParseUint(text, 10, bitSize) does,
// without a Go string, and reports false where that returns an error.
func parseDecimalUint(text []byte, bitSize uint) (uint64, bool) {
	if len(text) == 0 {
		return 0, false
	}
	maxValue := uint64(1)<<bitSize - 1
	var value uint64
	for _, c := range text {
		if c < '0' || c > '9' {
			return 0, false
		}
		digit := uint64(c - '0')
		if value > (maxValue-digit)/10 {
			return 0, false
		}
		value = value*10 + digit
	}
	return value, true
}

// parseDecimalInt parses text as strconv.ParseInt(text, 10, bitSize) does,
// without a Go string, and reports false where that returns an error.
func parseDecimalInt(text []byte, bitSize uint) (int64, bool) {
	negative := false
	if len(text) > 0 && (text[0] == '+' || text[0] == '-') {
		negative = text[0] == '-'
		text = text[1:]
	}
	magnitude, ok := parseDecimalUint(text, 64)
	cutoff := uint64(1) << (bitSize - 1)
	if !ok || (!negative && magnitude >= cutoff) || (negative && magnitude > cutoff) {
		return 0, false
	}
	if negative {
		return -int64(magnitude), true
	}
	return int64(magnitude), true
}
