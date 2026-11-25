package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/skycoin/skycoin-cx/t0-util" // thin: only FileError and its codes; no depth gain
)

// The trailer ends a file that cx build made: the core binary, the serialized
// program, then CoreTrailerSize bytes. The layout is in "Built programs" in
// docs/IR.md. All integers are little endian.
const (
	CoreTrailerSize          = 32
	CoreTrailerMagic         = "CXBUNDLE"
	CoreTrailerFormatVersion = 1
)

// CoreTrailer describes the program a trailer marks.
type CoreTrailer struct {
	CoreTrailerOffset   uint64 // offset of the program from the start of the file
	CoreTrailerLength   uint64 // length of the program in bytes
	CoreTrailerChecksum uint32 // CRC-32 (IEEE) of the program
}

// CoreTrailerAppend returns core followed by program and its trailer.
func CoreTrailerAppend(core []byte, program []byte) []byte {
	out := make([]byte, 0, len(core)+len(program)+CoreTrailerSize)
	out = append(out, core...)
	out = append(out, program...)
	out = binary.LittleEndian.AppendUint64(out, uint64(len(core)))
	out = binary.LittleEndian.AppendUint64(out, uint64(len(program)))
	out = binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(program))
	out = binary.LittleEndian.AppendUint32(out, CoreTrailerFormatVersion)
	return append(out, CoreTrailerMagic...)
}

// CoreTrailerDecode reads the trailer from tail, the last CoreTrailerSize
// bytes of a file of fileSize bytes. found is false when tail is no trailer.
// problem is not empty when it is one that does not fit the file.
func CoreTrailerDecode(tail []byte, fileSize uint64) (trailer CoreTrailer, found bool, problem string) {
	if len(tail) != CoreTrailerSize || !bytes.Equal(tail[24:], []byte(CoreTrailerMagic)) {
		return trailer, false, ""
	}
	trailer.CoreTrailerOffset = binary.LittleEndian.Uint64(tail[0:])
	trailer.CoreTrailerLength = binary.LittleEndian.Uint64(tail[8:])
	trailer.CoreTrailerChecksum = binary.LittleEndian.Uint32(tail[16:])
	if version := binary.LittleEndian.Uint32(tail[20:]); version != CoreTrailerFormatVersion {
		return trailer, true, fmt.Sprintf("trailer format %d, this core reads format %d", version, CoreTrailerFormatVersion)
	}
	end := trailer.CoreTrailerOffset + trailer.CoreTrailerLength
	if end < trailer.CoreTrailerOffset || end+CoreTrailerSize != fileSize {
		return trailer, true, fmt.Sprintf("trailer gives a program of %d bytes at %d, which does not end at the trailer of a %d byte file", trailer.CoreTrailerLength, trailer.CoreTrailerOffset, fileSize)
	}
	return trailer, true, ""
}

// CoreTrailerRead returns the program appended to the file at path, nil
// when the file ends in no trailer.
func CoreTrailerRead(path string) ([]byte, *util.FileError) {
	file, err := os.Open(path)
	if err != nil {
		return nil, util.FileErrorWrap(util.FileOperationOpen, path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, util.FileErrorWrap(util.FileOperationRead, path, err)
	}
	size := info.Size()
	if size < CoreTrailerSize {
		return nil, nil
	}
	tail := make([]byte, CoreTrailerSize)
	if _, err := file.ReadAt(tail, size-CoreTrailerSize); err != nil {
		return nil, util.FileErrorWrap(util.FileOperationRead, path, err)
	}
	trailer, found, problem := CoreTrailerDecode(tail, uint64(size))
	if !found {
		return nil, nil
	}
	if problem != "" {
		return nil, coreTrailerContentInvalid(path, problem)
	}
	program := make([]byte, trailer.CoreTrailerLength)
	if _, err := file.ReadAt(program, int64(trailer.CoreTrailerOffset)); err != nil && err != io.EOF {
		return nil, util.FileErrorWrap(util.FileOperationRead, path, err)
	}
	if crc32.ChecksumIEEE(program) != trailer.CoreTrailerChecksum {
		return nil, coreTrailerContentInvalid(path, "the appended program does not match its checksum")
	}
	return program, nil
}

func coreTrailerContentInvalid(path string, problem string) *util.FileError {
	return &util.FileError{
		FileErrorCode:      util.FileErrorCodeContentInvalid,
		FileErrorOperation: util.FileOperationRead,
		FileErrorMessage:   problem,
		FileErrorPath:      path,
	}
}
