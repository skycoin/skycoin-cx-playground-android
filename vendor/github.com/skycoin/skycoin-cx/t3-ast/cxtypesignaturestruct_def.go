package ast

type CXTypeSignature_Struct struct {
	// Fields stores what fields are being accessed from the
	// `CXArgument` and in what order. Whenever a `DEREF_FIELD` in
	// `DereferenceOperations` is found, we consume a field from
	// `Field` to determine the new offset to the desired
	// value.
	Fields []CXArgumentIndex

	// OBJMODEL: CXStructGid, phase 5 #337.
	// Now a pointer at rest, site S0272.
	StructType *CXStruct
}
