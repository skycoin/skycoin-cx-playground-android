package ast

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/skycoin/skycoin-cx/t0-types"
)

// serializeEncoder appends the primitives of the serialized form to a byte
// slice. Signed integers are zigzag varints and unsigned ones uvarints.
type serializeEncoder struct {
	serializeEncoderBytes []byte
}

func (encoder *serializeEncoder) serializeEncoderInt(value int64) {
	encoder.serializeEncoderBytes = binary.AppendVarint(encoder.serializeEncoderBytes, value)
}

func (encoder *serializeEncoder) serializeEncoderUint(value uint64) {
	encoder.serializeEncoderBytes = binary.AppendUvarint(encoder.serializeEncoderBytes, value)
}

func (encoder *serializeEncoder) serializeEncoderBool(value bool) {
	if value {
		encoder.serializeEncoderBytes = append(encoder.serializeEncoderBytes, 1)
	} else {
		encoder.serializeEncoderBytes = append(encoder.serializeEncoderBytes, 0)
	}
}

func (encoder *serializeEncoder) serializeEncoderPointer(value types.Pointer) {
	encoder.serializeEncoderUint(uint64(value))
}

// serializeEncoderLength writes the length of a slice or map, -1 for nil, so
// that nil and empty stay apart.
func (encoder *serializeEncoder) serializeEncoderLength(length int, isNil bool) {
	if isNil {
		encoder.serializeEncoderInt(-1)
		return
	}
	encoder.serializeEncoderInt(int64(length))
}

func (encoder *serializeEncoder) serializeEncoderByteSlice(value []byte) {
	encoder.serializeEncoderLength(len(value), value == nil)
	encoder.serializeEncoderBytes = append(encoder.serializeEncoderBytes, value...)
}

func (encoder *serializeEncoder) serializeEncoderInts(values []int) {
	encoder.serializeEncoderLength(len(values), values == nil)
	for _, value := range values {
		encoder.serializeEncoderInt(int64(value))
	}
}

func (encoder *serializeEncoder) serializeEncoderPointers(values []types.Pointer) {
	encoder.serializeEncoderLength(len(values), values == nil)
	for _, value := range values {
		encoder.serializeEncoderPointer(value)
	}
}

// serializeEncoderSection writes the bytes body appends as one section, its
// byte length first, so that a reader can find each section.
func (encoder *serializeEncoder) serializeEncoderSection(body func(section *serializeEncoder) error) error {
	var section serializeEncoder
	if err := body(&section); err != nil {
		return err
	}
	encoder.serializeEncoderUint(uint64(len(section.serializeEncoderBytes)))
	encoder.serializeEncoderBytes = append(encoder.serializeEncoderBytes, section.serializeEncoderBytes...)
	return nil
}

// errSerializeTruncated is the error for bytes that end inside a value.
var errSerializeTruncated = errors.New("serialized program is truncated")

// serializeDecoder reads what serializeEncoder wrote. The first error sticks,
// and every later read returns a zero value, so a caller checks once at the
// end of a section. No read panics on malformed bytes.
type serializeDecoder struct {
	serializeDecoderBytes    []byte
	serializeDecoderPosition int
	serializeDecoderError    error
}

func (decoder *serializeDecoder) serializeDecoderFail(err error) {
	if decoder.serializeDecoderError == nil {
		decoder.serializeDecoderError = err
	}
}

func (decoder *serializeDecoder) serializeDecoderFailf(format string, args ...interface{}) {
	decoder.serializeDecoderFail(fmt.Errorf("serialized program is malformed: "+format, args...))
}

func (decoder *serializeDecoder) serializeDecoderRemaining() int {
	return len(decoder.serializeDecoderBytes) - decoder.serializeDecoderPosition
}

func (decoder *serializeDecoder) serializeDecoderInt() int64 {
	if decoder.serializeDecoderError != nil {
		return 0
	}
	value, n := binary.Varint(decoder.serializeDecoderBytes[decoder.serializeDecoderPosition:])
	if n <= 0 {
		decoder.serializeDecoderFail(errSerializeTruncated)
		return 0
	}
	decoder.serializeDecoderPosition += n
	return value
}

func (decoder *serializeDecoder) serializeDecoderUint() uint64 {
	if decoder.serializeDecoderError != nil {
		return 0
	}
	value, n := binary.Uvarint(decoder.serializeDecoderBytes[decoder.serializeDecoderPosition:])
	if n <= 0 {
		decoder.serializeDecoderFail(errSerializeTruncated)
		return 0
	}
	decoder.serializeDecoderPosition += n
	return value
}

// serializeDecoderIntRange reads a signed integer and fails unless it is in
// minimum to maximum.
func (decoder *serializeDecoder) serializeDecoderIntRange(minimum int64, maximum int64, name string) int64 {
	value := decoder.serializeDecoderInt()
	if value < minimum || value > maximum {
		decoder.serializeDecoderFailf("%s %d is outside %d to %d", name, value, minimum, maximum)
		return 0
	}
	return value
}

func (decoder *serializeDecoder) serializeDecoderIntValue() int {
	return int(decoder.serializeDecoderIntRange(math.MinInt, math.MaxInt, "integer"))
}

func (decoder *serializeDecoder) serializeDecoderUint32() uint32 {
	value := decoder.serializeDecoderUint()
	if value > math.MaxUint32 {
		decoder.serializeDecoderFailf("value %d does not fit 32 bits", value)
		return 0
	}
	return uint32(value)
}

func (decoder *serializeDecoder) serializeDecoderBool() bool {
	if decoder.serializeDecoderError != nil {
		return false
	}
	if decoder.serializeDecoderRemaining() < 1 {
		decoder.serializeDecoderFail(errSerializeTruncated)
		return false
	}
	value := decoder.serializeDecoderBytes[decoder.serializeDecoderPosition]
	decoder.serializeDecoderPosition++
	if value > 1 {
		decoder.serializeDecoderFailf("boolean byte %d", value)
		return false
	}
	return value == 1
}

func (decoder *serializeDecoder) serializeDecoderPointer() types.Pointer {
	value := decoder.serializeDecoderUint()
	if value > uint64(types.InvalidPointer) {
		decoder.serializeDecoderFailf("pointer %d does not fit %d bytes", value, types.POINTER_SIZE)
		return 0
	}
	return types.Pointer(value)
}

// serializeDecoderLength reads a length written by serializeEncoderLength and
// returns -1 for nil. Each element takes at least elementSize bytes, so a
// length the remaining bytes cannot hold fails before anything is allocated.
func (decoder *serializeDecoder) serializeDecoderLength(elementSize int) int {
	length := decoder.serializeDecoderInt()
	if decoder.serializeDecoderError != nil {
		return 0
	}
	if length < -1 || length > int64(decoder.serializeDecoderRemaining()/elementSize) {
		decoder.serializeDecoderFailf("length %d with %d bytes left", length, decoder.serializeDecoderRemaining())
		return 0
	}
	return int(length)
}

func (decoder *serializeDecoder) serializeDecoderByteSlice() []byte {
	length := decoder.serializeDecoderLength(1)
	if length < 0 {
		return nil
	}
	value := make([]byte, length)
	copy(value, decoder.serializeDecoderBytes[decoder.serializeDecoderPosition:])
	decoder.serializeDecoderPosition += length
	return value
}

func (decoder *serializeDecoder) serializeDecoderInts() []int {
	length := decoder.serializeDecoderLength(1)
	if length < 0 {
		return nil
	}
	values := make([]int, length)
	for i := range values {
		values[i] = decoder.serializeDecoderIntValue()
	}
	return values
}

func (decoder *serializeDecoder) serializeDecoderPointers() []types.Pointer {
	length := decoder.serializeDecoderLength(1)
	if length < 0 {
		return nil
	}
	values := make([]types.Pointer, length)
	for i := range values {
		values[i] = decoder.serializeDecoderPointer()
	}
	return values
}

// serializeDecoderSection reads one section written by serializeEncoderSection
// with body, which must read all of it.
func (decoder *serializeDecoder) serializeDecoderSection(name string, body func(section *serializeDecoder)) {
	length := decoder.serializeDecoderUint()
	if decoder.serializeDecoderError != nil {
		return
	}
	if length > uint64(decoder.serializeDecoderRemaining()) {
		decoder.serializeDecoderFail(errSerializeTruncated)
		return
	}
	end := decoder.serializeDecoderPosition + int(length)
	section := serializeDecoder{serializeDecoderBytes: decoder.serializeDecoderBytes[decoder.serializeDecoderPosition:end]}
	body(&section)
	decoder.serializeDecoderPosition = end
	switch {
	case section.serializeDecoderError != nil:
		decoder.serializeDecoderFail(fmt.Errorf("%s section: %w", name, section.serializeDecoderError))
	case section.serializeDecoderRemaining() != 0:
		decoder.serializeDecoderFailf("%s section has %d bytes left over", name, section.serializeDecoderRemaining())
	}
}

func deserializeUint8(decoder *serializeDecoder) uint8 {
	value := decoder.serializeDecoderUint()
	if value > math.MaxUint8 {
		decoder.serializeDecoderFailf("value %d does not fit 8 bits", value)
		return 0
	}
	return uint8(value)
}
