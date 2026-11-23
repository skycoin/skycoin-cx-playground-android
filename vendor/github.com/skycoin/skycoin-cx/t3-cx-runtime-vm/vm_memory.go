package vm

import (
	"encoding/binary"
	"math"

	"github.com/skycoin/skycoin-cx/t0-types"
)

// The memory accessors read and write little endian values at a byte
// offset. Each checks the last byte once, so the loads combine into one and
// no slice capacity is needed.

func rBool(mem []byte, offset uint32) bool { return mem[offset] != 0 }

func wBool(mem []byte, offset uint32, value bool) {
	if value {
		mem[offset] = 1
	} else {
		mem[offset] = 0
	}
}

func rI8(mem []byte, offset uint32) int8      { return int8(mem[offset]) }
func wI8(mem []byte, offset uint32, v int8)   { mem[offset] = byte(v) }
func rUI8(mem []byte, offset uint32) uint8    { return mem[offset] }
func wUI8(mem []byte, offset uint32, v uint8) { mem[offset] = v }

func rUI16(mem []byte, offset uint32) uint16 {
	i := int(offset)
	return binary.LittleEndian.Uint16(mem[i : i+2])
}

func wUI16(mem []byte, offset uint32, v uint16) {
	i := int(offset)
	binary.LittleEndian.PutUint16(mem[i:i+2], v)
}

func rUI32(mem []byte, offset uint32) uint32 {
	i := int(offset)
	return binary.LittleEndian.Uint32(mem[i : i+4])
}

func wUI32(mem []byte, offset uint32, v uint32) {
	i := int(offset)
	binary.LittleEndian.PutUint32(mem[i:i+4], v)
}

func rUI64(mem []byte, offset uint32) uint64 {
	i := int(offset)
	return binary.LittleEndian.Uint64(mem[i : i+8])
}

func wUI64(mem []byte, offset uint32, v uint64) {
	i := int(offset)
	binary.LittleEndian.PutUint64(mem[i:i+8], v)
}

func rI16(mem []byte, offset uint32) int16      { return int16(rUI16(mem, offset)) }
func wI16(mem []byte, offset uint32, v int16)   { wUI16(mem, offset, uint16(v)) }
func rI32(mem []byte, offset uint32) int32      { return int32(rUI32(mem, offset)) }
func wI32(mem []byte, offset uint32, v int32)   { wUI32(mem, offset, uint32(v)) }
func rI64(mem []byte, offset uint32) int64      { return int64(rUI64(mem, offset)) }
func wI64(mem []byte, offset uint32, v int64)   { wUI64(mem, offset, uint64(v)) }
func rF32(mem []byte, offset uint32) float32    { return math.Float32frombits(rUI32(mem, offset)) }
func wF32(mem []byte, offset uint32, v float32) { wUI32(mem, offset, math.Float32bits(v)) }
func rF64(mem []byte, offset uint32) float64    { return math.Float64frombits(rUI64(mem, offset)) }
func wF64(mem []byte, offset uint32, v float64) { wUI64(mem, offset, math.Float64bits(v)) }

// memoryZero writes size zero bytes at offset without a call.
func memoryZero(mem []byte, offset uint32, size uint32) {
	for i := uint32(0); i+8 <= size; i += 8 {
		wUI64(mem, offset+i, 0)
	}
	for i := size &^ 7; i < size; i++ {
		mem[offset+i] = 0
	}
}

// rPointer reads a pointer of types.POINTER_SIZE bytes.
func rPointer(mem []byte, offset uint32) uint64 {
	if types.POINTER_SIZE == 4 {
		return uint64(rUI32(mem, offset))
	}
	return rUI64(mem, offset)
}

// wPointer writes address as a pointer of types.POINTER_SIZE bytes.
func wPointer(mem []byte, offset uint32, address uint32) {
	if types.POINTER_SIZE == 4 {
		wUI32(mem, offset, address)
		return
	}
	wUI64(mem, offset, uint64(address))
}

// memoryStrRead returns the bytes of the str at address as types.Read_str
// reads them, nil for a null str. It returns false when the object is not
// inside the first end bytes, where the old loop would read the heap or
// panic.
func memoryStrRead(mem []byte, address uint32, end uint32) ([]byte, bool) {
	object := rPointer(mem, address)
	if object == 0 || object == uint64(types.InvalidPointer) {
		return nil, true
	}
	if object+uint64(types.OBJECT_HEADER_SIZE) > uint64(end) {
		return nil, false
	}
	objectSize := uint64(types.Read_obj_size(mem, types.Pointer(object)))
	if objectSize < uint64(types.OBJECT_HEADER_SIZE) || object+objectSize > uint64(end) {
		return nil, false
	}
	return mem[object+uint64(types.OBJECT_HEADER_SIZE) : object+objectSize], true
}
