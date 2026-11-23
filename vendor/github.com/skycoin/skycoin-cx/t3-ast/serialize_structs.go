package ast

// serializeMagic starts every serialized program, so that a program appended
// to a file can be recognized. See "Serialized form" in docs/IR.md.
const serializeMagic = "CXPROGRM"

// serializeFormatVersion is the layout of the bytes after serializeMagic.
// It changes whenever a field is added, removed or reordered.
const serializeFormatVersion uint32 = 5

// serializeHeaderSize is serializeMagic, the format version, the pointer size
// and the language version, all fixed width little endian.
const serializeHeaderSize = len(serializeMagic) + 4 + 4 + 8

// SerializedDataSize is the byte size of each part of a serialized program.
// The parts add up to the whole, and each section counts its length prefix.
type SerializedDataSize struct {
	Header         int `json:"header"`
	Program        int `json:"program"`
	Strings        int `json:"strings"`
	Packages       int `json:"packages"`
	Structs        int `json:"structs"`
	Functions      int `json:"functions"`
	Expressions    int `json:"expressions"`
	Arguments      int `json:"arguments"`
	TypeSignatures int `json:"typeSignatures"`
	Memory         int `json:"memory"`
}
