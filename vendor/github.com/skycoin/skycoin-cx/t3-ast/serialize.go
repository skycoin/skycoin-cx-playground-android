package ast

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/skycoin/skycoin-cx/t0-types"
	"github.com/skycoin/skycoin-cx/t1-constants"
	objmodel "github.com/skycoin/skycoin-cx/t1-cx-core-objmodel"
	stringpool "github.com/skycoin/skycoin-cx/t2-cx-core-stringpool"
)

// SerializeCodec compresses the serialized bytes, as t0-lz4codec does. The
// interface keeps the compressor out of the packages that only load programs.
type SerializeCodec interface {
	Compress(src []byte) ([]byte, error)
	Decompress(src []byte) ([]byte, error)
}

// A serialized program is a snapshot of the arenas and managers of a
// compiled program, so that a deserialized program lowers and runs as the
// original does. The layout is in "Serialized form" in docs/IR.md.

// SerializeCXProgram translates cx program to slice of bytes that we can save.
// These slice of bytes can then be deserialize in the future and
// be translated back to cx program.
// includeMemory stores the program memory, without which a deserialized
// program gets zeroed memory of the same size. A nil codec leaves the bytes
// uncompressed. A non-nil codec must be initialized, and the bytes then
// include the memory and are compressed.
func SerializeCXProgram(prgrm *CXProgram, includeMemory bool, codec SerializeCodec) ([]byte, error) {
	b, _, err := encodeCXProgram(prgrm, includeMemory || codec != nil)
	if err != nil || codec == nil {
		return b, err
	}
	return compressCopy(codec, b)
}

// SerializeDebugInfo returns the byte size of each part of the uncompressed
// serialized program. wholeMemory gives the sizes of the bytes that
// compressed serialization compresses.
func SerializeDebugInfo(prgrm *CXProgram, includeMemory, wholeMemory bool) (SerializedDataSize, error) {
	_, sizes, err := encodeCXProgram(prgrm, includeMemory || wholeMemory)
	return sizes, err
}

// Deserialize deserializes a serialized CX program back to its golang struct representation.
// A nil codec reads b as uncompressed. A non-nil codec must be initialized.
// The program's CompileErrorPrint is left nil for the caller to set.
// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
// Now a constructor, site S0461.
// VIOLATION STRHOT-265: B, the serializer, compiler code run by an opcode, #412.
func Deserialize(b []byte, codec SerializeCodec) (*CXProgram, error) {
	if codec != nil {
		var err error
		if b, err = decompressCopy(codec, b); err != nil {
			return nil, err
		}
	}
	if err := SerializedHeaderCheck(b); err != nil {
		return nil, err
	}
	decoder := serializeDecoder{serializeDecoderBytes: b, serializeDecoderPosition: serializeHeaderSize}
	// OBJMODEL: state.CXProgramMng.CXProgramAlloc(), phase 4 #371.
	// Now a heap literal, site S0462.
	prgrm := &CXProgram{CxIrState: &CxIrState{}}
	prgrm.CxIrState.CxIrStateInit()
	prgrm.Version = int64(binary.LittleEndian.Uint64(b[len(serializeMagic)+8:]))
	var calls []serializeCallReference
	decoder.serializeDecoderSection("program", func(section *serializeDecoder) { calls = deserializeProgramFields(prgrm, section) })
	decoder.serializeDecoderSection("strings", func(section *serializeDecoder) { deserializeStrings(prgrm, section) })
	decoder.serializeDecoderSection("packages", func(section *serializeDecoder) { deserializePackages(prgrm, section) })
	decoder.serializeDecoderSection("structs", func(section *serializeDecoder) { deserializeStructs(prgrm, section) })
	decoder.serializeDecoderSection("functions", func(section *serializeDecoder) { deserializeFunctions(prgrm, section) })
	decoder.serializeDecoderSection("expressions", func(section *serializeDecoder) { deserializeExpressions(prgrm, section) })
	decoder.serializeDecoderSection("arguments", func(section *serializeDecoder) { deserializeArguments(prgrm, section) })
	decoder.serializeDecoderSection("type signatures", func(section *serializeDecoder) { deserializeTypeSignatures(prgrm, section) })
	decoder.serializeDecoderSection("memory", func(section *serializeDecoder) { deserializeMemory(prgrm, section) })
	if decoder.serializeDecoderError == nil && decoder.serializeDecoderRemaining() != 0 {
		decoder.serializeDecoderFailf("%d bytes after the last section", decoder.serializeDecoderRemaining())
	}
	deserializeCallStack(prgrm, &decoder, calls)
	if decoder.serializeDecoderError != nil {
		return nil, decoder.serializeDecoderError
	}
	return prgrm, nil
}

// SerializedHeaderCheck returns nil when b starts with the header of a
// program this cx can load: the magic, this format version, this pointer size
// and this language version. Otherwise it says which one differs.
// VIOLATION STRHOT-310: B, the serializer, compiler code run by an opcode, #412.
func SerializedHeaderCheck(b []byte) error {
	if len(b) < serializeHeaderSize || string(b[:len(serializeMagic)]) != serializeMagic {
		return fmt.Errorf("not a serialized CX program")
	}
	position := len(serializeMagic)
	if format := binary.LittleEndian.Uint32(b[position:]); format != serializeFormatVersion {
		return fmt.Errorf("serialized program has format %d, this cx reads format %d, recompile it from source", format, serializeFormatVersion)
	}
	if pointerSize := binary.LittleEndian.Uint32(b[position+4:]); pointerSize != uint32(types.POINTER_SIZE) {
		return fmt.Errorf("serialized program was compiled with %d byte pointers, this cx uses %d byte pointers, load it with a cx built to match (the ptr64 build tag gives 8)", pointerSize, types.POINTER_SIZE)
	}
	if version := int64(binary.LittleEndian.Uint64(b[position+8:])); version != constants.LanguageVersionCurrent {
		return fmt.Errorf("serialized program is language version %d, this CX implements version %d, recompile it from source", version, constants.LanguageVersionCurrent)
	}
	return nil
}

// encodeCXProgram serializes prgrm without compression and returns the size
// of each part.
func encodeCXProgram(prgrm *CXProgram, includeMemory bool) ([]byte, SerializedDataSize, error) {
	var sizes SerializedDataSize
	if err := serializeUnusedManagersCheck(prgrm.CxIrState); err != nil {
		return nil, sizes, err
	}
	encoder := serializeEncoder{serializeEncoderBytes: make([]byte, 0, 4096)}
	encoder.serializeEncoderBytes = append(encoder.serializeEncoderBytes, serializeMagic...)
	encoder.serializeEncoderBytes = binary.LittleEndian.AppendUint32(encoder.serializeEncoderBytes, serializeFormatVersion)
	encoder.serializeEncoderBytes = binary.LittleEndian.AppendUint32(encoder.serializeEncoderBytes, uint32(types.POINTER_SIZE))
	encoder.serializeEncoderBytes = binary.LittleEndian.AppendUint64(encoder.serializeEncoderBytes, uint64(prgrm.Version))
	sizes.Header = len(encoder.serializeEncoderBytes)

	sections := []struct {
		size *int
		body func(section *serializeEncoder) error
	}{
		{&sizes.Program, func(section *serializeEncoder) error { return serializeProgramFields(prgrm, section) }},
		{&sizes.Strings, func(section *serializeEncoder) error { serializeStrings(prgrm, section); return nil }},
		{&sizes.Packages, func(section *serializeEncoder) error { return serializePackages(prgrm, section) }},
		{&sizes.Structs, func(section *serializeEncoder) error { serializeStructs(prgrm, section); return nil }},
		{&sizes.Functions, func(section *serializeEncoder) error { return serializeFunctions(prgrm, section) }},
		{&sizes.Expressions, func(section *serializeEncoder) error { return serializeExpressions(prgrm, section) }},
		{&sizes.Arguments, func(section *serializeEncoder) error { return serializeArguments(prgrm, section) }},
		{&sizes.TypeSignatures, func(section *serializeEncoder) error { return serializeTypeSignatures(prgrm, section) }},
		{&sizes.Memory, func(section *serializeEncoder) error { serializeMemory(prgrm, section, includeMemory); return nil }},
	}
	for _, section := range sections {
		before := len(encoder.serializeEncoderBytes)
		if err := encoder.serializeEncoderSection(section.body); err != nil {
			return nil, sizes, err
		}
		*section.size = len(encoder.serializeEncoderBytes) - before
	}
	return encoder.serializeEncoderBytes, sizes, nil
}

// serializeUnusedManagersCheck fails when a manager that no compile stage
// fills yet holds an object, since the serialized form does not carry it.
// VIOLATION STRHOT-312: E, error or assert text, built on a failure branch only, #412.
func serializeUnusedManagersCheck(state *CxIrState) error {
	unused := []struct {
		typeTag objmodel.TypeTag
		used    bool
	}{
		{CXProgramGidTypeTag, state.CXProgramMng.CXProgramMngNext != 0},
		{CXPackageGidTypeTag, state.CXPackageMng.CXPackageMngNext != 0},
		{CXFunctionGidTypeTag, state.CXFunctionMng.CXFunctionMngNext != 0},
		{CXNativeFunctionGidTypeTag, state.CXNativeFunctionMng.CXNativeFunctionMngNext != 0},
		{CXStructGidTypeTag, state.CXStructMng.CXStructMngNext != 0},
		{CXExpressionGidTypeTag, state.CXExpressionMng.CXExpressionMngNext != 0},
		{CXAtomicOperatorGidTypeTag, state.CXAtomicOperatorMng.CXAtomicOperatorMngNext != 0},
		{CXCallGidTypeTag, state.CXCallMng.CXCallMngNext != 0},
	}
	for _, manager := range unused {
		if manager.used {
			return fmt.Errorf("cannot serialize the program: %sMng holds objects and the serialized form does not carry them yet", objmodel.GidTypeTagNameTable[manager.typeTag])
		}
	}
	return nil
}

// compressCopy compresses b with codec and copies the result out of the
// codec's buffer.
func compressCopy(codec SerializeCodec, b []byte) ([]byte, error) {
	out, err := codec.Compress(b)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), out...), nil
}

// decompressCopy decompresses b with codec and copies the result out of the
// codec's buffer.
func decompressCopy(codec SerializeCodec, b []byte) ([]byte, error) {
	out, err := codec.Decompress(b)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), out...), nil
}

// ----------------------------------------------------------------
//                     References between objects

// serializeStructReference returns the index in prgrm.CXStructs of the struct
// a pointer field names, or -1 for nil.
// VIOLATION STRHOT-311: E, error or assert text, built on a failure branch only, #412.
func serializeStructReference(prgrm *CXProgram, strct *CXStruct) (int64, error) {
	if strct == nil {
		return -1, nil
	}
	if strct.Index >= 0 && strct.Index < len(prgrm.CXStructs) {
		arena := &prgrm.CXStructs[strct.Index]
		if arena == strct || arena.Name == strct.Name && arena.Package == strct.Package {
			return int64(strct.Index), nil
		}
	}
	return 0, fmt.Errorf("cannot serialize the program: struct %q is referenced but is not in CXStructs", prgrm.CXProgramNameText(strct.Name))
}

// VIOLATION STRHOT-307: B, the serializer, compiler code run by an opcode, #412.
func deserializeStructReference(prgrm *CXProgram, decoder *serializeDecoder) *CXStruct {
	index := decoder.serializeDecoderIntRange(-1, int64(len(prgrm.CXStructs))-1, "struct reference")
	if index < 0 || decoder.serializeDecoderError != nil {
		return nil
	}
	// OBJMODEL: state.CXStructMng.CXStructGet(gid), phase 5 #337.
	// Now a pointer into an arena, as compiled programs hold it.
	return &prgrm.CXStructs[index]
}

// serializeOwnedStruct writes a struct that a function, an operator or a
// package owns through a pointer, such as its inputs or globals.
func serializeOwnedStruct(encoder *serializeEncoder, strct *CXStruct) {
	encoder.serializeEncoderBool(strct != nil)
	if strct != nil {
		serializeStruct(encoder, strct)
	}
}

func deserializeOwnedStruct(decoder *serializeDecoder) *CXStruct {
	if !decoder.serializeDecoderBool() {
		return nil
	}
	// OBJMODEL: state.CXStructMng.CXStructAlloc(), phase 4 #336.
	// Now a heap literal, as compiled programs hold it.
	strct := &CXStruct{}
	deserializeStruct(decoder, strct)
	return strct
}

func serializeStruct(encoder *serializeEncoder, strct *CXStruct) {
	encoder.serializeEncoderInt(int64(strct.Index))
	encoder.serializeEncoderInt(int64(strct.Name))
	encoder.serializeEncoderInt(int64(strct.Package))
	encoder.serializeEncoderLength(len(strct.Fields), strct.Fields == nil)
	for _, field := range strct.Fields {
		encoder.serializeEncoderInt(int64(field))
	}
	encoder.serializeEncoderInt(int64(strct.FileName))
	encoder.serializeEncoderInt(int64(strct.FileLine))
	encoder.serializeEncoderInt(int64(strct.FileColumn))
}

func deserializeStruct(decoder *serializeDecoder, strct *CXStruct) {
	strct.Index = decoder.serializeDecoderIntValue()
	strct.Name = stringpool.StringGid(decoder.serializeDecoderInt())
	strct.Package = CXPackageIndex(decoder.serializeDecoderIntValue())
	if length := decoder.serializeDecoderLength(1); length >= 0 {
		strct.Fields = make([]CXTypeSignatureIndex, length)
		for i := range strct.Fields {
			strct.Fields[i] = CXTypeSignatureIndex(decoder.serializeDecoderIntValue())
		}
	}
	strct.FileName = SourceFileGid(decoder.serializeDecoderInt())
	strct.FileLine = decoder.serializeDecoderIntValue()
	strct.FileColumn = decoder.serializeDecoderIntValue()
}

// ----------------------------------------------------------------
//                     Managers

// serializeManagerSlots is the bookkeeping of one manager besides its
// objects, see docs/ObjectModel.md.
type serializeManagerSlots struct {
	serializeManagerSlotsCapacity    int
	serializeManagerSlotsGenerations []uint32
	serializeManagerSlotsLive        []bool
	serializeManagerSlotsFree        []uint32
	serializeManagerSlotsNext        uint32
	serializeManagerSlotsRetired     uint32
	serializeManagerSlotsLen         int
	serializeManagerSlotsGrowable    bool
}

// serializeManager writes a manager's slots up to Next and its live objects,
// so that every Gid stays valid after deserializing.
func serializeManager[Object any](encoder *serializeEncoder, objects []Object, slots serializeManagerSlots, objectEncode func(object *Object) error) error {
	next := int(slots.serializeManagerSlotsNext)
	encoder.serializeEncoderUint(uint64(slots.serializeManagerSlotsCapacity))
	encoder.serializeEncoderUint(uint64(next))
	encoder.serializeEncoderUint(uint64(slots.serializeManagerSlotsRetired))
	encoder.serializeEncoderUint(uint64(slots.serializeManagerSlotsLen))
	encoder.serializeEncoderBool(slots.serializeManagerSlotsGrowable)
	encoder.serializeEncoderLength(len(slots.serializeManagerSlotsFree), slots.serializeManagerSlotsFree == nil)
	for _, free := range slots.serializeManagerSlotsFree {
		encoder.serializeEncoderUint(uint64(free))
	}
	for index := 0; index < next; index++ {
		encoder.serializeEncoderUint(uint64(slots.serializeManagerSlotsGenerations[index]))
		live := slots.serializeManagerSlotsLive[index]
		encoder.serializeEncoderBool(live)
		if live {
			if err := objectEncode(&objects[index]); err != nil {
				return err
			}
		}
	}
	return nil
}

// deserializeManager reads what serializeManager wrote and checks that the
// slot counts agree, the invariant every manager call asserts.
// VIOLATION STRHOT-302: B, the serializer, compiler code run by an opcode, #412.
func deserializeManager[Object any](decoder *serializeDecoder, name string, objectDecode func(object *Object)) ([]Object, serializeManagerSlots) {
	var slots serializeManagerSlots
	capacity := decoder.serializeDecoderUint()
	next := decoder.serializeDecoderUint()
	slots.serializeManagerSlotsRetired = decoder.serializeDecoderUint32()
	length := decoder.serializeDecoderUint()
	slots.serializeManagerSlotsGrowable = decoder.serializeDecoderBool()
	freeLength := decoder.serializeDecoderLength(1)
	if decoder.serializeDecoderError != nil {
		return nil, slots
	}
	free := uint64(max(freeLength, 0))
	// Each slot up to Next takes at least two bytes, a free slot one.
	if capacity > objmodel.GidCapMax || next > capacity || length > next || free > next ||
		next > uint64(decoder.serializeDecoderRemaining()/2) || capacity > next+1<<20 ||
		length != next-free-uint64(slots.serializeManagerSlotsRetired) {
		decoder.serializeDecoderFailf("%s has Cap %d, Next %d, Len %d, %d free and %d retired slots", name, capacity, next, length, free, slots.serializeManagerSlotsRetired)
		return nil, slots
	}
	slots.serializeManagerSlotsCapacity = int(capacity)
	slots.serializeManagerSlotsNext = uint32(next)
	slots.serializeManagerSlotsLen = int(length)
	if freeLength >= 0 {
		slots.serializeManagerSlotsFree = make([]uint32, free)
	}
	for i := range slots.serializeManagerSlotsFree {
		slot := decoder.serializeDecoderUint32()
		if uint64(slot) >= next {
			decoder.serializeDecoderFailf("%s free slot %d is not below Next %d", name, slot, next)
			return nil, slots
		}
		slots.serializeManagerSlotsFree[i] = slot
	}
	objects := make([]Object, capacity)
	slots.serializeManagerSlotsGenerations = make([]uint32, capacity)
	slots.serializeManagerSlotsLive = make([]bool, capacity)
	live := uint64(0)
	for index := uint64(0); index < next && decoder.serializeDecoderError == nil; index++ {
		slots.serializeManagerSlotsGenerations[index] = decoder.serializeDecoderUint32()
		if decoder.serializeDecoderBool() {
			slots.serializeManagerSlotsLive[index] = true
			live++
			objectDecode(&objects[index])
		}
	}
	if decoder.serializeDecoderError == nil && live != length {
		decoder.serializeDecoderFailf("%s has %d live slots, Len %d", name, live, length)
	}
	return objects, slots
}

// ----------------------------------------------------------------
//                     Program section

// serializeCallReference is a call stack entry whose operator is a function
// index, resolved once every section is read.
type serializeCallReference struct {
	serializeCallReferenceOperator     int64
	serializeCallReferenceLine         int
	serializeCallReferenceFramePointer types.Pointer
}

func serializeProgramFields(prgrm *CXProgram, encoder *serializeEncoder) error {
	encoder.serializeEncoderPointer(prgrm.Stack.Size)
	encoder.serializeEncoderPointer(prgrm.Stack.StartsAt)
	encoder.serializeEncoderPointer(prgrm.Stack.Pointer)
	encoder.serializeEncoderPointer(prgrm.Data.Size)
	encoder.serializeEncoderPointer(prgrm.Data.StartsAt)
	encoder.serializeEncoderPointer(prgrm.Heap.Size)
	encoder.serializeEncoderPointer(prgrm.Heap.StartsAt)
	encoder.serializeEncoderPointer(prgrm.Heap.Pointer)

	names := make([]stringpool.StringGid, 0, len(prgrm.Packages))
	for name := range prgrm.Packages {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	encoder.serializeEncoderLength(len(names), prgrm.Packages == nil)
	for _, name := range names {
		encoder.serializeEncoderInt(int64(name))
		encoder.serializeEncoderInt(int64(prgrm.Packages[name]))
	}
	encoder.serializeEncoderInt(int64(prgrm.CurrentPackage))

	encoder.serializeEncoderLength(len(prgrm.ProgramInput), prgrm.ProgramInput == nil)
	for _, input := range prgrm.ProgramInput {
		encoder.serializeEncoderInt(int64(input))
	}

	// Only the entries up to the last one in use are written.
	used := 0
	for i, call := range prgrm.CallStack {
		if call != (CXCall{}) {
			used = i + 1
		}
	}
	encoder.serializeEncoderUint(uint64(len(prgrm.CallStack)))
	encoder.serializeEncoderPointer(prgrm.CallCounter)
	encoder.serializeEncoderUint(uint64(used))
	for _, call := range prgrm.CallStack[:used] {
		operator := int64(-1)
		if call.Operator != nil {
			operator = int64(call.Operator.Index)
		}
		encoder.serializeEncoderInt(operator)
		encoder.serializeEncoderInt(int64(call.Line))
		encoder.serializeEncoderPointer(call.FramePointer)
	}
	encoder.serializeEncoderBool(prgrm.Terminated)
	serializeLibraries(encoder, &prgrm.ProgramLibrariesUsed)
	serializeLibraries(encoder, &prgrm.ProgramLibrariesEnabled)

	encoder.serializeEncoderLength(len(prgrm.SysInitExprs), prgrm.SysInitExprs == nil)
	for i := range prgrm.SysInitExprs {
		serializeExpression(encoder, &prgrm.SysInitExprs[i])
	}
	return nil
}

// VIOLATION STRHOT-304: B, the serializer, compiler code run by an opcode, #412.
func deserializeProgramFields(prgrm *CXProgram, decoder *serializeDecoder) []serializeCallReference {
	prgrm.Stack.Size = decoder.serializeDecoderPointer()
	prgrm.Stack.StartsAt = decoder.serializeDecoderPointer()
	prgrm.Stack.Pointer = decoder.serializeDecoderPointer()
	prgrm.Data.Size = decoder.serializeDecoderPointer()
	prgrm.Data.StartsAt = decoder.serializeDecoderPointer()
	prgrm.Heap.Size = decoder.serializeDecoderPointer()
	prgrm.Heap.StartsAt = decoder.serializeDecoderPointer()
	prgrm.Heap.Pointer = decoder.serializeDecoderPointer()

	if length := decoder.serializeDecoderLength(2); length >= 0 {
		prgrm.Packages = make(map[stringpool.StringGid]CXPackageIndex, length)
		for i := 0; i < length; i++ {
			name := stringpool.StringGid(decoder.serializeDecoderInt())
			prgrm.Packages[name] = CXPackageIndex(decoder.serializeDecoderIntValue())
		}
	}
	prgrm.CurrentPackage = CXPackageIndex(decoder.serializeDecoderIntValue())

	if length := decoder.serializeDecoderLength(1); length >= 0 {
		prgrm.ProgramInput = make([]CXArgumentIndex, length)
		for i := range prgrm.ProgramInput {
			prgrm.ProgramInput[i] = CXArgumentIndex(decoder.serializeDecoderIntValue())
		}
	}

	stackLength := decoder.serializeDecoderUint()
	prgrm.CallCounter = decoder.serializeDecoderPointer()
	used := decoder.serializeDecoderUint()
	if decoder.serializeDecoderError != nil {
		return nil
	}
	if stackLength > uint64(constants.CALLSTACK_SIZE) || used > stackLength || used > uint64(decoder.serializeDecoderRemaining()/3) {
		decoder.serializeDecoderFailf("call stack of %d entries with %d in use", stackLength, used)
		return nil
	}
	// OBJMODEL: state.CXCallMng (manager storage) or []CXCallGid, phase 4 #366.
	// Now a slice of objects, as cxinit.MakeProgram makes it.
	prgrm.CallStack = make([]CXCall, stackLength)
	calls := make([]serializeCallReference, used)
	for i := range calls {
		calls[i].serializeCallReferenceOperator = decoder.serializeDecoderInt()
		calls[i].serializeCallReferenceLine = decoder.serializeDecoderIntValue()
		calls[i].serializeCallReferenceFramePointer = decoder.serializeDecoderPointer()
	}
	prgrm.Terminated = decoder.serializeDecoderBool()
	deserializeLibraries(decoder, &prgrm.ProgramLibrariesUsed)
	deserializeLibraries(decoder, &prgrm.ProgramLibrariesEnabled)

	if length := decoder.serializeDecoderLength(5); length >= 0 {
		prgrm.SysInitExprs = make([]CXExpression, length)
		for i := range prgrm.SysInitExprs {
			deserializeExpression(decoder, &prgrm.SysInitExprs[i])
		}
	}
	return calls
}

// deserializeCallStack points the call stack entries at their functions,
// which are read after the program section.
// VIOLATION STRHOT-299: B, the serializer, compiler code run by an opcode, #412.
func deserializeCallStack(prgrm *CXProgram, decoder *serializeDecoder, calls []serializeCallReference) {
	if decoder.serializeDecoderError != nil {
		return
	}
	for i, call := range calls {
		operator := call.serializeCallReferenceOperator
		if operator < -1 || operator >= int64(len(prgrm.CXFunctions)) {
			decoder.serializeDecoderFailf("call %d names function %d of %d", i, operator, len(prgrm.CXFunctions))
			return
		}
		if operator >= 0 {
			// OBJMODEL: state.CXFunctionMng.CXFunctionGet(gid), phase 5 #352.
			// Now a pointer into an arena, as the call stack holds it.
			prgrm.CallStack[i].Operator = &prgrm.CXFunctions[operator]
		}
		prgrm.CallStack[i].Line = call.serializeCallReferenceLine
		prgrm.CallStack[i].FramePointer = call.serializeCallReferenceFramePointer
	}
}

// ----------------------------------------------------------------
//                     Strings section

func serializeStrings(prgrm *CXProgram, encoder *serializeEncoder) {
	state := prgrm.CxIrState
	pool := &state.StringPool
	encoder.serializeEncoderUint(uint64(pool.StringPoolLen()))
	for slot := 0; slot < pool.StringPoolLen(); slot++ {
		gid := stringpool.StringGid(objmodel.GidCompose(stringpool.StringGidTypeTag, objmodel.GidGenerationFirst, uint32(slot)))
		encoder.serializeEncoderByteSlice(pool.StringPoolBytes(gid))
	}
	encoder.serializeEncoderBool(pool.StringPoolFrozen)
	encoder.serializeEncoderInt(int64(state.CxIrStateNameMain))
	encoder.serializeEncoderInt(int64(state.CxIrStateNameSysInit))
	encoder.serializeEncoderInt(int64(state.CxIrStateNameOsArgs))
	encoder.serializeEncoderInt(int64(state.CxIrStateNameOs))
	encoder.serializeEncoderUint(uint64(len(state.SourceFileMng.SourceFileMngPaths)))
	for _, path := range state.SourceFileMng.SourceFileMngPaths {
		encoder.serializeEncoderInt(int64(path))
	}
}

// deserializeStrings interns the strings in their slot order, which gives
// every string the Gid it had.
// VIOLATION STRHOT-306: B, the serializer, compiler code run by an opcode, #412.
func deserializeStrings(prgrm *CXProgram, decoder *serializeDecoder) {
	state := prgrm.CxIrState
	count := decoder.serializeDecoderUint()
	if decoder.serializeDecoderError != nil {
		return
	}
	if count > uint64(decoder.serializeDecoderRemaining()) {
		decoder.serializeDecoderFail(errSerializeTruncated)
		return
	}
	state.StringPool.StringPoolInit(max(int(count), 1024), max(decoder.serializeDecoderRemaining(), 16384))
	for slot := uint64(0); slot < count && decoder.serializeDecoderError == nil; slot++ {
		text := decoder.serializeDecoderByteSlice()
		if decoder.serializeDecoderError != nil {
			return
		}
		if _, found := state.StringPool.StringPoolFind(text); found {
			decoder.serializeDecoderFailf("string slot %d repeats an earlier string or is empty", slot)
			return
		}
		state.StringPool.StringPoolIntern(text)
	}
	frozen := decoder.serializeDecoderBool()
	names := []*stringpool.StringGid{&state.CxIrStateNameMain, &state.CxIrStateNameSysInit, &state.CxIrStateNameOsArgs, &state.CxIrStateNameOs}
	for _, name := range names {
		*name = deserializeStringGid(decoder, &state.StringPool)
	}
	paths := decoder.serializeDecoderUint()
	if decoder.serializeDecoderError != nil {
		return
	}
	if paths > uint64(decoder.serializeDecoderRemaining()) {
		decoder.serializeDecoderFail(errSerializeTruncated)
		return
	}
	state.SourceFileMng.SourceFileMngPaths = make([]stringpool.StringGid, paths)
	for i := range state.SourceFileMng.SourceFileMngPaths {
		state.SourceFileMng.SourceFileMngPaths[i] = deserializeStringGid(decoder, &state.StringPool)
	}
	if frozen {
		state.StringPool.StringPoolFreeze()
	}
}

// deserializeStringGid reads a string Gid and checks that it names a slot.
// VIOLATION STRHOT-305: B, the serializer, compiler code run by an opcode, #412.
func deserializeStringGid(decoder *serializeDecoder, pool *stringpool.StringPool) stringpool.StringGid {
	gid := decoder.serializeDecoderInt()
	if gid == int64(stringpool.StringGidNil) {
		return stringpool.StringGidNil
	}
	if objmodel.GidTypeTag(gid) != stringpool.StringGidTypeTag || objmodel.GidGeneration(gid) != objmodel.GidGenerationFirst ||
		uint64(objmodel.GidIndex(gid)) >= uint64(pool.StringPoolLen()) {
		decoder.serializeDecoderFailf("string Gid %#x", gid)
		return stringpool.StringGidNil
	}
	return stringpool.StringGid(gid)
}

// ----------------------------------------------------------------
//                     Packages section

func serializePackages(prgrm *CXProgram, encoder *serializeEncoder) error {
	encoder.serializeEncoderLength(len(prgrm.CXPackages), prgrm.CXPackages == nil)
	for i := range prgrm.CXPackages {
		pkg := &prgrm.CXPackages[i]
		encoder.serializeEncoderInt(int64(pkg.Name))
		encoder.serializeEncoderInt(int64(pkg.Index))
		serializeNameMap(encoder, pkg.Imports)
		serializeNameMap(encoder, pkg.Functions)
		serializeNameMap(encoder, pkg.Structs)
		keys := make([]CXPackageMethodKey, 0, len(pkg.Methods))
		for key := range pkg.Methods {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].CXPackageMethodKeyStruct != keys[j].CXPackageMethodKeyStruct {
				return keys[i].CXPackageMethodKeyStruct < keys[j].CXPackageMethodKeyStruct
			}
			return keys[i].CXPackageMethodKeyMethod < keys[j].CXPackageMethodKeyMethod
		})
		encoder.serializeEncoderLength(len(keys), pkg.Methods == nil)
		for _, key := range keys {
			encoder.serializeEncoderInt(int64(key.CXPackageMethodKeyStruct))
			encoder.serializeEncoderInt(int64(key.CXPackageMethodKeyMethod))
			encoder.serializeEncoderInt(int64(pkg.Methods[key]))
		}
		serializeOwnedStruct(encoder, pkg.Globals)
		encoder.serializeEncoderInt(int64(pkg.CurrentFunction))
	}
	return nil
}

func deserializePackages(prgrm *CXProgram, decoder *serializeDecoder) {
	length := decoder.serializeDecoderLength(8)
	if length < 0 {
		return
	}
	// OBJMODEL: state.CXPackageMng (manager storage) or []CXPackageGid, phase 4 #361.
	// Now a slice of objects, the arena of a compiled program.
	prgrm.CXPackages = make([]CXPackage, length)
	for i := range prgrm.CXPackages {
		pkg := &prgrm.CXPackages[i]
		pkg.Name = stringpool.StringGid(decoder.serializeDecoderInt())
		pkg.Index = decoder.serializeDecoderIntValue()
		pkg.Imports = deserializeNameMap[CXPackageIndex](decoder)
		pkg.Functions = deserializeNameMap[CXFunctionIndex](decoder)
		pkg.Structs = deserializeNameMap[CXStructIndex](decoder)
		if count := decoder.serializeDecoderLength(3); count >= 0 {
			pkg.Methods = make(map[CXPackageMethodKey]CXFunctionIndex, count)
			for j := 0; j < count; j++ {
				var key CXPackageMethodKey
				key.CXPackageMethodKeyStruct = stringpool.StringGid(decoder.serializeDecoderInt())
				key.CXPackageMethodKeyMethod = stringpool.StringGid(decoder.serializeDecoderInt())
				pkg.Methods[key] = CXFunctionIndex(decoder.serializeDecoderIntValue())
			}
		}
		pkg.Globals = deserializeOwnedStruct(decoder)
		pkg.CurrentFunction = CXFunctionIndex(decoder.serializeDecoderIntValue())
	}
}

// serializeNameMap writes a map from names to indexes in name Gid order.
func serializeNameMap[Index ~int](encoder *serializeEncoder, names map[stringpool.StringGid]Index) {
	keys := make([]stringpool.StringGid, 0, len(names))
	for key := range names {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	encoder.serializeEncoderLength(len(keys), names == nil)
	for _, key := range keys {
		encoder.serializeEncoderInt(int64(key))
		encoder.serializeEncoderInt(int64(names[key]))
	}
}

func deserializeNameMap[Index ~int](decoder *serializeDecoder) map[stringpool.StringGid]Index {
	length := decoder.serializeDecoderLength(2)
	if length < 0 {
		return nil
	}
	names := make(map[stringpool.StringGid]Index, length)
	for i := 0; i < length; i++ {
		key := stringpool.StringGid(decoder.serializeDecoderInt())
		names[key] = Index(decoder.serializeDecoderIntValue())
	}
	return names
}

// ----------------------------------------------------------------
//                     Structs section

func serializeStructs(prgrm *CXProgram, encoder *serializeEncoder) {
	encoder.serializeEncoderLength(len(prgrm.CXStructs), prgrm.CXStructs == nil)
	for i := range prgrm.CXStructs {
		serializeStruct(encoder, &prgrm.CXStructs[i])
	}
}

func deserializeStructs(prgrm *CXProgram, decoder *serializeDecoder) {
	length := decoder.serializeDecoderLength(4)
	if length < 0 {
		return
	}
	// OBJMODEL: state.CXStructMng (manager storage) or []CXStructGid, phase 4 #336.
	// Now a slice of objects, the arena of a compiled program.
	prgrm.CXStructs = make([]CXStruct, length)
	for i := range prgrm.CXStructs {
		deserializeStruct(decoder, &prgrm.CXStructs[i])
	}
}

// ----------------------------------------------------------------
//                     Functions section

func serializeFunctions(prgrm *CXProgram, encoder *serializeEncoder) error {
	encoder.serializeEncoderLength(len(prgrm.CXFunctions), prgrm.CXFunctions == nil)
	for i := range prgrm.CXFunctions {
		if err := serializeFunction(prgrm, encoder, &prgrm.CXFunctions[i]); err != nil {
			return err
		}
	}
	return nil
}

// GCREACH: writes the GC root tables ListOfPointers and ListOfSlots,
// see docs/GarbageCollector.md.
func serializeFunction(prgrm *CXProgram, encoder *serializeEncoder, fn *CXFunction) error {
	encoder.serializeEncoderInt(int64(fn.Index))
	encoder.serializeEncoderInt(int64(fn.Name))
	encoder.serializeEncoderInt(int64(fn.Package))
	encoder.serializeEncoderInt(int64(fn.AtomicOPCode))
	encoder.serializeEncoderInt(int64(fn.LibraryNumber))
	encoder.serializeEncoderInt(int64(fn.LibraryFunctionNumber))
	serializeOwnedStruct(encoder, fn.Inputs)
	serializeOwnedStruct(encoder, fn.Outputs)
	encoder.serializeEncoderLength(len(fn.Expressions), fn.Expressions == nil)
	for i := range fn.Expressions {
		serializeExpression(encoder, &fn.Expressions[i])
	}
	encoder.serializeEncoderLength(len(fn.LocalVariables), fn.LocalVariables == nil)
	for _, name := range fn.LocalVariables {
		encoder.serializeEncoderInt(int64(name))
	}
	encoder.serializeEncoderInt(int64(fn.LineCount))
	encoder.serializeEncoderPointer(fn.Size)
	encoder.serializeEncoderInt(int64(fn.FileName))
	encoder.serializeEncoderInt(int64(fn.FileLine))
	encoder.serializeEncoderInt(int64(fn.FileColumn))
	encoder.serializeEncoderLength(len(fn.ListOfPointers), fn.ListOfPointers == nil)
	for _, arg := range fn.ListOfPointers {
		encoder.serializeEncoderBool(arg != nil)
		if arg != nil {
			if err := serializeArgument(prgrm, encoder, arg); err != nil {
				return err
			}
		}
	}
	encoder.serializeEncoderLength(len(fn.EscapeInputs), fn.EscapeInputs == nil)
	for _, escape := range fn.EscapeInputs {
		encoder.serializeEncoderUint(uint64(escape))
	}
	encoder.serializeEncoderLength(len(fn.ListOfSlots), fn.ListOfSlots == nil)
	for i := range fn.ListOfSlots {
		slot := &fn.ListOfSlots[i]
		encoder.serializeEncoderPointer(slot.CXFunctionSlotOffset)
		encoder.serializeEncoderPointers(slot.CXFunctionSlotLengths)
		encoder.serializeEncoderInt(int64(slot.CXFunctionSlotType))
		encoder.serializeEncoderInts(slot.CXFunctionSlotSpecifiers)
		strct, err := serializeStructReference(prgrm, slot.CXFunctionSlotStruct)
		if err != nil {
			return err
		}
		encoder.serializeEncoderInt(strct)
	}
	return nil
}

func deserializeFunctions(prgrm *CXProgram, decoder *serializeDecoder) {
	length := decoder.serializeDecoderLength(16)
	if length < 0 {
		return
	}
	// OBJMODEL: state.CXFunctionMng (manager storage) or []CXFunctionGid, phase 4 #351.
	// Now a slice of objects, the arena of a compiled program.
	prgrm.CXFunctions = make([]CXFunction, length)
	for i := range prgrm.CXFunctions {
		deserializeFunction(prgrm, decoder, &prgrm.CXFunctions[i])
	}
}

// GCREACH: reads the GC root tables ListOfPointers and ListOfSlots,
// see docs/GarbageCollector.md.
// VIOLATION STRHOT-063: B, the serializer, compiler code run by an opcode, #412.
func deserializeFunction(prgrm *CXProgram, decoder *serializeDecoder, fn *CXFunction) {
	fn.Index = decoder.serializeDecoderIntValue()
	fn.Name = stringpool.StringGid(decoder.serializeDecoderInt())
	fn.Package = CXPackageIndex(decoder.serializeDecoderIntValue())
	fn.AtomicOPCode = decoder.serializeDecoderIntValue()
	fn.LibraryNumber = decoder.serializeDecoderIntValue()
	fn.LibraryFunctionNumber = decoder.serializeDecoderIntValue()
	if fn.AtomicOPCode == constants.OP_LIBRARY_CALL && (fn.LibraryNumber <= 0 || fn.LibraryNumber >= constants.LibraryCount || fn.LibraryFunctionNumber <= 0) {
		decoder.serializeDecoderFailf("function %d calls library %d function %d, which does not exist", fn.Index, fn.LibraryNumber, fn.LibraryFunctionNumber)
	}
	fn.Inputs = deserializeOwnedStruct(decoder)
	fn.Outputs = deserializeOwnedStruct(decoder)
	if length := decoder.serializeDecoderLength(5); length >= 0 {
		fn.Expressions = make([]CXExpression, length)
		for i := range fn.Expressions {
			deserializeExpression(decoder, &fn.Expressions[i])
		}
	}
	if length := decoder.serializeDecoderLength(1); length >= 0 {
		fn.LocalVariables = make([]stringpool.StringGid, length)
		for i := range fn.LocalVariables {
			fn.LocalVariables[i] = stringpool.StringGid(decoder.serializeDecoderInt())
		}
	}
	fn.LineCount = decoder.serializeDecoderIntValue()
	fn.Size = decoder.serializeDecoderPointer()
	fn.FileName = SourceFileGid(decoder.serializeDecoderInt())
	fn.FileLine = decoder.serializeDecoderIntValue()
	fn.FileColumn = decoder.serializeDecoderIntValue()
	if length := decoder.serializeDecoderLength(1); length >= 0 {
		// OBJMODEL: state.CXArgumentMng (manager storage) or []CXArgumentGid, phase 4 #326.
		// Now a slice of pointers, as compiled programs hold the GC roots.
		fn.ListOfPointers = make([]*CXArgument, length)
		for i := range fn.ListOfPointers {
			if decoder.serializeDecoderBool() {
				// OBJMODEL: state.CXArgumentMng.CXArgumentAlloc(), phase 4 #326.
				// Now a heap literal, as compiled programs hold the GC roots.
				fn.ListOfPointers[i] = &CXArgument{}
				deserializeArgument(prgrm, decoder, fn.ListOfPointers[i])
			}
		}
	}
	if length := decoder.serializeDecoderLength(1); length >= 0 {
		fn.EscapeInputs = make([]CXFunctionEscape, length)
		for i := range fn.EscapeInputs {
			escape := decoder.serializeDecoderUint()
			if escape > uint64(CXFunctionEscapeHeap) {
				decoder.serializeDecoderFailf("escape %d", escape)
				return
			}
			fn.EscapeInputs[i] = CXFunctionEscape(escape)
		}
	}
	if length := decoder.serializeDecoderLength(5); length >= 0 {
		fn.ListOfSlots = make([]CXFunctionSlot, length)
		for i := range fn.ListOfSlots {
			slot := &fn.ListOfSlots[i]
			slot.CXFunctionSlotOffset = decoder.serializeDecoderPointer()
			slot.CXFunctionSlotLengths = decoder.serializeDecoderPointers()
			slot.CXFunctionSlotType = types.Code(decoder.serializeDecoderIntValue())
			slot.CXFunctionSlotSpecifiers = decoder.serializeDecoderInts()
			slot.CXFunctionSlotStruct = deserializeStructReference(prgrm, decoder)
		}
	}
}

// ----------------------------------------------------------------
//                     Expressions section

func serializeExpression(encoder *serializeEncoder, expr *CXExpression) {
	encoder.serializeEncoderInt(int64(expr.ExpressionType))
	encoder.serializeEncoderUint(uint64(expr.ExpressionScopeOpenCount))
	encoder.serializeEncoderUint(uint64(expr.ExpressionScopeCloseCount))
	encoder.serializeEncoderInt(int64(expr.Index))
	encoder.serializeEncoderInt(int64(expr.Type))
}

func deserializeExpression(decoder *serializeDecoder, expr *CXExpression) {
	expr.ExpressionType = CXEXPR_TYPE(decoder.serializeDecoderIntValue())
	expr.ExpressionScopeOpenCount = deserializeUint8(decoder)
	expr.ExpressionScopeCloseCount = deserializeUint8(decoder)
	expr.Index = decoder.serializeDecoderIntValue()
	expr.Type = CXOPERATION_TYPE(decoder.serializeDecoderIntValue())
}

func serializeExpressions(prgrm *CXProgram, encoder *serializeEncoder) error {
	encoder.serializeEncoderLength(len(prgrm.CXAtomicOps), prgrm.CXAtomicOps == nil)
	for i := range prgrm.CXAtomicOps {
		op := &prgrm.CXAtomicOps[i]
		serializeOwnedStruct(encoder, op.Inputs)
		serializeOwnedStruct(encoder, op.Outputs)
		encoder.serializeEncoderInt(int64(op.Operator))
		encoder.serializeEncoderInt(int64(op.Function))
		encoder.serializeEncoderInt(int64(op.Package))
		encoder.serializeEncoderInt(int64(op.Label))
		encoder.serializeEncoderInt(int64(op.ThenLines))
		encoder.serializeEncoderInt(int64(op.ElseLines))
	}
	manager := &prgrm.CxIrState.CXLineMng
	return serializeManager(encoder, manager.CXLineMngObjects, serializeManagerSlots{
		len(manager.CXLineMngObjects), manager.CXLineMngGenerations, manager.CXLineMngLive, manager.CXLineMngFree,
		manager.CXLineMngNext, manager.CXLineMngRetired, manager.CXLineMngLen, manager.CXLineMngGrowable,
	}, func(line *CXLine) error {
		encoder.serializeEncoderInt(int64(line.FileName))
		encoder.serializeEncoderInt(int64(line.LineNumber))
		encoder.serializeEncoderInt(int64(line.LineColumn))
		encoder.serializeEncoderInt(int64(line.LineStr))
		return nil
	})
}

// VIOLATION STRHOT-300: B, the serializer, compiler code run by an opcode, #412.
func deserializeExpressions(prgrm *CXProgram, decoder *serializeDecoder) {
	if length := decoder.serializeDecoderLength(8); length >= 0 {
		// OBJMODEL: state.CXAtomicOperatorMng (manager storage) or []CXAtomicOperatorGid, phase 4 #341.
		// Now a slice of objects, the arena of a compiled program.
		prgrm.CXAtomicOps = make([]CXAtomicOperator, length)
		for i := range prgrm.CXAtomicOps {
			op := &prgrm.CXAtomicOps[i]
			op.Inputs = deserializeOwnedStruct(decoder)
			op.Outputs = deserializeOwnedStruct(decoder)
			op.Operator = CXFunctionIndex(decoder.serializeDecoderIntValue())
			op.Function = CXFunctionIndex(decoder.serializeDecoderIntValue())
			op.Package = CXPackageIndex(decoder.serializeDecoderIntValue())
			op.Label = stringpool.StringGid(decoder.serializeDecoderInt())
			op.ThenLines = decoder.serializeDecoderIntValue()
			op.ElseLines = decoder.serializeDecoderIntValue()
		}
	}
	objects, slots := deserializeManager(decoder, "CXLineMng", func(line *CXLine) {
		line.FileName = SourceFileGid(decoder.serializeDecoderInt())
		line.LineNumber = decoder.serializeDecoderIntValue()
		line.LineColumn = decoder.serializeDecoderIntValue()
		line.LineStr = stringpool.StringGid(decoder.serializeDecoderInt())
	})
	if decoder.serializeDecoderError != nil {
		return
	}
	prgrm.CxIrState.CXLineMng = CXLineMng{
		objects, slots.serializeManagerSlotsGenerations, slots.serializeManagerSlotsLive, slots.serializeManagerSlotsFree,
		slots.serializeManagerSlotsNext, slots.serializeManagerSlotsRetired, slots.serializeManagerSlotsLen, slots.serializeManagerSlotsGrowable,
	}
}

// ----------------------------------------------------------------
//                     Arguments section

func serializeArgument(prgrm *CXProgram, encoder *serializeEncoder, arg *CXArgument) error {
	encoder.serializeEncoderInt(int64(arg.Name))
	encoder.serializeEncoderInt(int64(arg.Index))
	encoder.serializeEncoderInt(int64(arg.Package))
	encoder.serializeEncoderPointers(arg.Lengths)
	encoder.serializeEncoderInts(arg.DereferenceOperations)
	encoder.serializeEncoderInts(arg.DeclarationSpecifiers)
	encoder.serializeEncoderLength(len(arg.Indexes), arg.Indexes == nil)
	for _, index := range arg.Indexes {
		encoder.serializeEncoderInt(int64(index))
	}
	for _, list := range [][]CXArgumentIndex{arg.Fields, arg.Inputs, arg.Outputs} {
		encoder.serializeEncoderLength(len(list), list == nil)
		for _, index := range list {
			encoder.serializeEncoderInt(int64(index))
		}
	}
	encoder.serializeEncoderInt(int64(arg.Type))
	encoder.serializeEncoderInt(int64(arg.PointerTargetType))
	encoder.serializeEncoderPointer(arg.Size)
	encoder.serializeEncoderPointer(arg.Offset)
	encoder.serializeEncoderInt(int64(arg.PassBy))
	encoder.serializeEncoderInt(int64(arg.ArgDetails))
	strct, err := serializeStructReference(prgrm, arg.StructType)
	if err != nil {
		return err
	}
	encoder.serializeEncoderInt(strct)
	encoder.serializeEncoderBool(arg.PreviouslyDeclared)
	return nil
}

func deserializeArgument(prgrm *CXProgram, decoder *serializeDecoder, arg *CXArgument) {
	arg.Name = stringpool.StringGid(decoder.serializeDecoderInt())
	arg.Index = decoder.serializeDecoderIntValue()
	arg.Package = CXPackageIndex(decoder.serializeDecoderIntValue())
	arg.Lengths = decoder.serializeDecoderPointers()
	arg.DereferenceOperations = decoder.serializeDecoderInts()
	arg.DeclarationSpecifiers = decoder.serializeDecoderInts()
	if length := decoder.serializeDecoderLength(1); length >= 0 {
		arg.Indexes = make([]CXTypeSignatureIndex, length)
		for i := range arg.Indexes {
			arg.Indexes[i] = CXTypeSignatureIndex(decoder.serializeDecoderIntValue())
		}
	}
	for _, list := range []*[]CXArgumentIndex{&arg.Fields, &arg.Inputs, &arg.Outputs} {
		if length := decoder.serializeDecoderLength(1); length >= 0 {
			*list = make([]CXArgumentIndex, length)
			for i := range *list {
				(*list)[i] = CXArgumentIndex(decoder.serializeDecoderIntValue())
			}
		}
	}
	arg.Type = types.Code(decoder.serializeDecoderIntValue())
	arg.PointerTargetType = types.Code(decoder.serializeDecoderIntValue())
	arg.Size = decoder.serializeDecoderPointer()
	arg.Offset = decoder.serializeDecoderPointer()
	arg.PassBy = decoder.serializeDecoderIntValue()
	arg.ArgDetails = CXArgumentDebugGid(decoder.serializeDecoderInt())
	arg.StructType = deserializeStructReference(prgrm, decoder)
	arg.PreviouslyDeclared = decoder.serializeDecoderBool()
}

func serializeArguments(prgrm *CXProgram, encoder *serializeEncoder) error {
	arguments := &prgrm.CxIrState.CXArgumentMng
	err := serializeManager(encoder, arguments.CXArgumentMngObjects, serializeManagerSlots{
		len(arguments.CXArgumentMngObjects), arguments.CXArgumentMngGenerations, arguments.CXArgumentMngLive, arguments.CXArgumentMngFree,
		arguments.CXArgumentMngNext, arguments.CXArgumentMngRetired, arguments.CXArgumentMngLen, arguments.CXArgumentMngGrowable,
	}, func(arg *CXArgument) error { return serializeArgument(prgrm, encoder, arg) })
	if err != nil {
		return err
	}
	debug := &prgrm.CxIrState.CXArgumentDebugMng
	return serializeManager(encoder, debug.CXArgumentDebugMngObjects, serializeManagerSlots{
		len(debug.CXArgumentDebugMngObjects), debug.CXArgumentDebugMngGenerations, debug.CXArgumentDebugMngLive, debug.CXArgumentDebugMngFree,
		debug.CXArgumentDebugMngNext, debug.CXArgumentDebugMngRetired, debug.CXArgumentDebugMngLen, debug.CXArgumentDebugMngGrowable,
	}, func(details *CXArgumentDebug) error {
		encoder.serializeEncoderInt(int64(details.FileName))
		encoder.serializeEncoderInt(int64(details.FileLine))
		encoder.serializeEncoderInt(int64(details.FileColumn))
		return nil
	})
}

// VIOLATION STRHOT-298: B, the serializer, compiler code run by an opcode, #412.
func deserializeArguments(prgrm *CXProgram, decoder *serializeDecoder) {
	arguments, slots := deserializeManager(decoder, "CXArgumentMng", func(arg *CXArgument) { deserializeArgument(prgrm, decoder, arg) })
	if decoder.serializeDecoderError != nil {
		return
	}
	prgrm.CxIrState.CXArgumentMng = CXArgumentMng{
		arguments, slots.serializeManagerSlotsGenerations, slots.serializeManagerSlotsLive, slots.serializeManagerSlotsFree,
		slots.serializeManagerSlotsNext, slots.serializeManagerSlotsRetired, slots.serializeManagerSlotsLen, slots.serializeManagerSlotsGrowable,
	}
	debug, slots := deserializeManager(decoder, "CXArgumentDebugMng", func(details *CXArgumentDebug) {
		details.FileName = SourceFileGid(decoder.serializeDecoderInt())
		details.FileLine = decoder.serializeDecoderIntValue()
		details.FileColumn = decoder.serializeDecoderIntValue()
	})
	if decoder.serializeDecoderError != nil {
		return
	}
	prgrm.CxIrState.CXArgumentDebugMng = CXArgumentDebugMng{
		debug, slots.serializeManagerSlotsGenerations, slots.serializeManagerSlotsLive, slots.serializeManagerSlotsFree,
		slots.serializeManagerSlotsNext, slots.serializeManagerSlotsRetired, slots.serializeManagerSlotsLen, slots.serializeManagerSlotsGrowable,
	}
}

// ----------------------------------------------------------------
//                     Type signatures section

func serializeTypeSignatures(prgrm *CXProgram, encoder *serializeEncoder) error {
	state := prgrm.CxIrState
	signatures := &state.CXTypeSignatureMng
	err := serializeManager(encoder, signatures.CXTypeSignatureMngObjects, serializeManagerSlots{
		len(signatures.CXTypeSignatureMngObjects), signatures.CXTypeSignatureMngGenerations, signatures.CXTypeSignatureMngLive, signatures.CXTypeSignatureMngFree,
		signatures.CXTypeSignatureMngNext, signatures.CXTypeSignatureMngRetired, signatures.CXTypeSignatureMngLen, signatures.CXTypeSignatureMngGrowable,
	}, func(signature *CXTypeSignature) error {
		encoder.serializeEncoderInt(int64(signature.Index))
		encoder.serializeEncoderInt(int64(signature.Name))
		encoder.serializeEncoderPointer(signature.Offset)
		encoder.serializeEncoderInt(int64(signature.Type))
		encoder.serializeEncoderInt(int64(signature.Package))
		encoder.serializeEncoderInt(int64(signature.PassBy))
		encoder.serializeEncoderBool(signature.IsDeref)
		encoder.serializeEncoderInt(int64(signature.Meta))
		encoder.serializeEncoderInt(int64(signature.ArgDetails))
		return nil
	})
	if err != nil {
		return err
	}
	arrays := &state.CXTypeSignature_ArrayMng
	err = serializeManager(encoder, arrays.CXTypeSignature_ArrayMngObjects, serializeManagerSlots{
		len(arrays.CXTypeSignature_ArrayMngObjects), arrays.CXTypeSignature_ArrayMngGenerations, arrays.CXTypeSignature_ArrayMngLive, arrays.CXTypeSignature_ArrayMngFree,
		arrays.CXTypeSignature_ArrayMngNext, arrays.CXTypeSignature_ArrayMngRetired, arrays.CXTypeSignature_ArrayMngLen, arrays.CXTypeSignature_ArrayMngGrowable,
	}, func(array *CXTypeSignature_Array) error {
		encoder.serializeEncoderInt(int64(array.Meta))
		encoder.serializeEncoderPointers(array.Lengths)
		encoder.serializeEncoderLength(len(array.Indexes), array.Indexes == nil)
		for _, index := range array.Indexes {
			encoder.serializeEncoderInt(int64(index))
		}
		return nil
	})
	if err != nil {
		return err
	}
	structs := &state.CXTypeSignature_StructMng
	return serializeManager(encoder, structs.CXTypeSignature_StructMngObjects, serializeManagerSlots{
		len(structs.CXTypeSignature_StructMngObjects), structs.CXTypeSignature_StructMngGenerations, structs.CXTypeSignature_StructMngLive, structs.CXTypeSignature_StructMngFree,
		structs.CXTypeSignature_StructMngNext, structs.CXTypeSignature_StructMngRetired, structs.CXTypeSignature_StructMngLen, structs.CXTypeSignature_StructMngGrowable,
	}, func(signature *CXTypeSignature_Struct) error {
		encoder.serializeEncoderLength(len(signature.Fields), signature.Fields == nil)
		for _, field := range signature.Fields {
			encoder.serializeEncoderInt(int64(field))
		}
		strct, err := serializeStructReference(prgrm, signature.StructType)
		encoder.serializeEncoderInt(strct)
		return err
	})
}

// VIOLATION STRHOT-308: B, the serializer, compiler code run by an opcode, #412.
func deserializeTypeSignatures(prgrm *CXProgram, decoder *serializeDecoder) {
	state := prgrm.CxIrState
	signatures, slots := deserializeManager(decoder, "CXTypeSignatureMng", func(signature *CXTypeSignature) {
		signature.Index = CXTypeSignatureIndex(decoder.serializeDecoderIntValue())
		signature.Name = stringpool.StringGid(decoder.serializeDecoderInt())
		signature.Offset = decoder.serializeDecoderPointer()
		signature.Type = CXTypeSignature_TYPE(decoder.serializeDecoderIntValue())
		signature.Package = CXPackageIndex(decoder.serializeDecoderIntValue())
		signature.PassBy = decoder.serializeDecoderIntValue()
		signature.IsDeref = decoder.serializeDecoderBool()
		signature.Meta = decoder.serializeDecoderIntValue()
		signature.ArgDetails = CXArgumentDebugGid(decoder.serializeDecoderInt())
	})
	if decoder.serializeDecoderError != nil {
		return
	}
	state.CXTypeSignatureMng = CXTypeSignatureMng{
		signatures, slots.serializeManagerSlotsGenerations, slots.serializeManagerSlotsLive, slots.serializeManagerSlotsFree,
		slots.serializeManagerSlotsNext, slots.serializeManagerSlotsRetired, slots.serializeManagerSlotsLen, slots.serializeManagerSlotsGrowable,
	}
	arrays, slots := deserializeManager(decoder, "CXTypeSignature_ArrayMng", func(array *CXTypeSignature_Array) {
		array.Meta = decoder.serializeDecoderIntValue()
		array.Lengths = decoder.serializeDecoderPointers()
		if length := decoder.serializeDecoderLength(1); length >= 0 {
			array.Indexes = make([]CXTypeSignatureIndex, length)
			for i := range array.Indexes {
				array.Indexes[i] = CXTypeSignatureIndex(decoder.serializeDecoderIntValue())
			}
		}
	})
	if decoder.serializeDecoderError != nil {
		return
	}
	state.CXTypeSignature_ArrayMng = CXTypeSignature_ArrayMng{
		arrays, slots.serializeManagerSlotsGenerations, slots.serializeManagerSlotsLive, slots.serializeManagerSlotsFree,
		slots.serializeManagerSlotsNext, slots.serializeManagerSlotsRetired, slots.serializeManagerSlotsLen, slots.serializeManagerSlotsGrowable,
	}
	structs, slots := deserializeManager(decoder, "CXTypeSignature_StructMng", func(signature *CXTypeSignature_Struct) {
		if length := decoder.serializeDecoderLength(1); length >= 0 {
			signature.Fields = make([]CXArgumentIndex, length)
			for i := range signature.Fields {
				signature.Fields[i] = CXArgumentIndex(decoder.serializeDecoderIntValue())
			}
		}
		signature.StructType = deserializeStructReference(prgrm, decoder)
	})
	if decoder.serializeDecoderError != nil {
		return
	}
	state.CXTypeSignature_StructMng = CXTypeSignature_StructMng{
		structs, slots.serializeManagerSlotsGenerations, slots.serializeManagerSlotsLive, slots.serializeManagerSlotsFree,
		slots.serializeManagerSlotsNext, slots.serializeManagerSlotsRetired, slots.serializeManagerSlotsLen, slots.serializeManagerSlotsGrowable,
	}
}

// ----------------------------------------------------------------
//                     Memory section

// serializeMemory writes the size of the memory and, when includeMemory is
// set, the bytes from its first to its last nonzero byte.
func serializeMemory(prgrm *CXProgram, encoder *serializeEncoder, includeMemory bool) {
	encoder.serializeEncoderUint(uint64(len(prgrm.Memory)))
	encoder.serializeEncoderBool(includeMemory)
	if !includeMemory {
		return
	}
	first, last := 0, len(prgrm.Memory)
	for first < last && prgrm.Memory[first] == 0 {
		first++
	}
	for last > first && prgrm.Memory[last-1] == 0 {
		last--
	}
	encoder.serializeEncoderUint(uint64(first))
	encoder.serializeEncoderByteSlice(prgrm.Memory[first:last])
}

// VIOLATION STRHOT-303: B, the serializer, compiler code run by an opcode, #412.
func deserializeMemory(prgrm *CXProgram, decoder *serializeDecoder) {
	size := decoder.serializeDecoderUint()
	included := decoder.serializeDecoderBool()
	if decoder.serializeDecoderError != nil {
		return
	}
	limit := uint64(prgrm.Heap.StartsAt) + uint64(prgrm.Heap.Size)
	if size > uint64(types.InvalidPointer) || size > limit+uint64(prgrm.Stack.Size)+uint64(prgrm.Data.Size) {
		decoder.serializeDecoderFailf("memory of %d bytes for a stack of %d, data of %d and a heap of %d", size, prgrm.Stack.Size, prgrm.Data.Size, prgrm.Heap.Size)
		return
	}
	prgrm.Memory = make([]byte, size)
	if !included {
		return
	}
	first := decoder.serializeDecoderUint()
	stored := decoder.serializeDecoderByteSlice()
	if decoder.serializeDecoderError == nil && (first > size || uint64(len(stored)) > size-first) {
		decoder.serializeDecoderFailf("%d memory bytes at %d do not fit %d", len(stored), first, size)
		return
	}
	copy(prgrm.Memory[first:], stored)
}

// serializeLibraries writes a set of system libraries as its length and one
// bool per library number.
func serializeLibraries(encoder *serializeEncoder, libraries *[constants.LibraryCount]bool) {
	encoder.serializeEncoderUint(uint64(len(libraries)))
	for _, marked := range libraries {
		encoder.serializeEncoderBool(marked)
	}
}

// deserializeLibraries reads a set that serializeLibraries wrote. A program
// with another number of libraries fails.
// VIOLATION STRHOT-301: B, the serializer, compiler code run by an opcode, #412.
func deserializeLibraries(decoder *serializeDecoder, libraries *[constants.LibraryCount]bool) {
	if count := decoder.serializeDecoderUint(); decoder.serializeDecoderError == nil && count != uint64(len(libraries)) {
		decoder.serializeDecoderFailf("%d system libraries, this cx has %d", count, len(libraries))
		return
	}
	for library := range libraries {
		libraries[library] = decoder.serializeDecoderBool()
	}
}
