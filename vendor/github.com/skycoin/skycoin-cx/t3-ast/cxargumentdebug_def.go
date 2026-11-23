package ast

// CXArgumentDebug is the source position of an argument. A FileColumn of 0
// is unknown.
type CXArgumentDebug struct {
	FileName   SourceFileGid
	FileLine   int
	FileColumn int
}
