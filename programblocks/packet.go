package programblocks

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Packet constants, see "UDP operations" in docs/ProgramDistribution.md.
const (
	ProtocolVersion    = 1
	DefaultPacketSize  = 1200
	SmallPacketSize    = 1024
	PacketHeaderLength = 6
	ReplyFlag          = 0x80
	MaxErrorTextLength = 100
)

// Operation is the operation code of a packet.
type Operation byte

// The operations. A reply carries the request's code plus ReplyFlag, or
// OperationError.
const (
	OperationListManifests Operation = 0x01
	OperationGetManifest   Operation = 0x02
	OperationGetBlockList  Operation = 0x03
	OperationGetBlocks     Operation = 0x04
	OperationError         Operation = 0xFF
)

// ErrorCode is the code of an error reply.
type ErrorCode byte

// The error codes.
const (
	ErrorCodeUnknownManifest    ErrorCode = 1
	ErrorCodeRangeOutside       ErrorCode = 2
	ErrorCodeMalformedRequest   ErrorCode = 3
	ErrorCodeUnsupportedVersion ErrorCode = 4
)

// The body lengths that bound each reply's payload.
const (
	listReplyLength         = PacketHeaderLength + 32 + 4
	manifestReplyFixed      = PacketHeaderLength + 32 + 4 + 4
	blockListReplyFixed     = PacketHeaderLength + 32 + 4 + 2
	blocksReplyFixed        = PacketHeaderLength + 32 + 8 + 2
	errorReplyFixed         = PacketHeaderLength + 2
	addressedRequestLength  = PacketHeaderLength + 32 + 4 + 2
	getBlocksRequestLength  = PacketHeaderLength + 32 + 8 + 2
	listManifestsReqLength  = PacketHeaderLength
	maxRequestPacketLength  = getBlocksRequestLength
	minimumUsefulPacketSize = blocksReplyFixed + DefaultBlockSize
)

// MaxManifestPayload is the most manifest bytes one reply of packetSize holds.
func MaxManifestPayload(packetSize int) int { return packetSize - manifestReplyFixed }

// MaxBlockListEntries is the most block list entries one reply holds.
func MaxBlockListEntries(packetSize int) int {
	return (packetSize - blockListReplyFixed) / BlockEntryLength
}

// MaxBlocksPayload is the most linear packing bytes one reply holds.
func MaxBlocksPayload(packetSize int) int { return packetSize - blocksReplyFixed }

// CheckPacketSize returns an error for a packet size that cannot carry one
// 512-byte block.
func CheckPacketSize(packetSize int) error {
	if packetSize < minimumUsefulPacketSize || packetSize > 65507 {
		return fmt.Errorf("packet size %d out of range %d to 65507", packetSize, minimumUsefulPacketSize)
	}
	return nil
}

// Request is a decoded request. Offset is the manifest offset, the first
// block ID or the linear packing offset, and Length the byte length or the
// entry count, as the operation defines.
type Request struct {
	Operation Operation
	RequestID uint32
	Address   Hash
	Offset    uint64
	Length    uint16
}

// Reply is a decoded reply. Total is the manifest length for list manifests
// and get manifest. Length is the entry count for get block list.
type Reply struct {
	Operation Operation
	RequestID uint32
	Address   Hash
	Offset    uint64
	Total     uint32
	Length    uint16
	Payload   []byte
	ErrorCode ErrorCode
	ErrorText string
}

// ErrPacket marks a packet that cannot be decoded.
var ErrPacket = errors.New("malformed packet")

func putHeader(b []byte, op Operation, id uint32) {
	b[0] = ProtocolVersion
	b[1] = byte(op)
	binary.BigEndian.PutUint32(b[2:], id)
}

// EncodeRequest returns the bytes of r.
func EncodeRequest(r Request) []byte {
	be := binary.BigEndian
	switch r.Operation {
	case OperationListManifests:
		b := make([]byte, listManifestsReqLength)
		putHeader(b, r.Operation, r.RequestID)
		return b
	case OperationGetBlocks:
		b := make([]byte, getBlocksRequestLength)
		putHeader(b, r.Operation, r.RequestID)
		copy(b[6:], r.Address[:])
		be.PutUint64(b[38:], r.Offset)
		be.PutUint16(b[46:], r.Length)
		return b
	default:
		b := make([]byte, addressedRequestLength)
		putHeader(b, r.Operation, r.RequestID)
		copy(b[6:], r.Address[:])
		be.PutUint32(b[38:], uint32(r.Offset))
		be.PutUint16(b[42:], r.Length)
		return b
	}
}

// DecodeRequest parses a request. An error that comes with a request ID
// still lets the server answer with an error reply.
func DecodeRequest(b []byte) (Request, error) {
	var r Request
	if len(b) < PacketHeaderLength {
		return r, ErrPacket
	}
	be := binary.BigEndian
	r.Operation, r.RequestID = Operation(b[1]), be.Uint32(b[2:])
	if b[0] != ProtocolVersion {
		return r, fmt.Errorf("%w: version %d", ErrPacket, b[0])
	}
	want := map[Operation]int{OperationListManifests: listManifestsReqLength, OperationGetManifest: addressedRequestLength,
		OperationGetBlockList: addressedRequestLength, OperationGetBlocks: getBlocksRequestLength}[r.Operation]
	if want == 0 || len(b) != want {
		return r, fmt.Errorf("%w: operation %#x with %d bytes", ErrPacket, byte(r.Operation), len(b))
	}
	if r.Operation == OperationListManifests {
		return r, nil
	}
	copy(r.Address[:], b[6:38])
	if r.Operation == OperationGetBlocks {
		r.Offset, r.Length = be.Uint64(b[38:]), be.Uint16(b[46:])
	} else {
		r.Offset, r.Length = uint64(be.Uint32(b[38:])), be.Uint16(b[42:])
	}
	return r, nil
}

// EncodeReply returns the bytes of r, or an error when they would exceed
// packetSize.
func EncodeReply(r Reply, packetSize int) ([]byte, error) {
	be := binary.BigEndian
	var b []byte
	switch r.Operation {
	case OperationListManifests | ReplyFlag:
		b = make([]byte, listReplyLength)
		copy(b[6:], r.Address[:])
		be.PutUint32(b[38:], r.Total)
	case OperationGetManifest | ReplyFlag:
		b = make([]byte, manifestReplyFixed, manifestReplyFixed+len(r.Payload))
		copy(b[6:], r.Address[:])
		be.PutUint32(b[38:], uint32(r.Offset))
		be.PutUint32(b[42:], r.Total)
		b = append(b, r.Payload...)
	case OperationGetBlockList | ReplyFlag:
		if len(r.Payload) != int(r.Length)*BlockEntryLength {
			return nil, fmt.Errorf("block list reply of %d entries with %d bytes", r.Length, len(r.Payload))
		}
		b = make([]byte, blockListReplyFixed, blockListReplyFixed+len(r.Payload))
		copy(b[6:], r.Address[:])
		be.PutUint32(b[38:], uint32(r.Offset))
		be.PutUint16(b[42:], r.Length)
		b = append(b, r.Payload...)
	case OperationGetBlocks | ReplyFlag:
		b = make([]byte, blocksReplyFixed, blocksReplyFixed+len(r.Payload))
		copy(b[6:], r.Address[:])
		be.PutUint64(b[38:], r.Offset)
		be.PutUint16(b[46:], uint16(len(r.Payload)))
		b = append(b, r.Payload...)
	case OperationError:
		text := r.ErrorText[:min(len(r.ErrorText), MaxErrorTextLength)]
		b = make([]byte, errorReplyFixed, errorReplyFixed+len(text))
		b[6], b[7] = byte(r.ErrorCode), byte(len(text))
		b = append(b, text...)
	default:
		return nil, fmt.Errorf("no reply operation %#x", byte(r.Operation))
	}
	putHeader(b, r.Operation, r.RequestID)
	if len(b) > packetSize {
		return nil, fmt.Errorf("reply of %d bytes exceeds the packet size %d", len(b), packetSize)
	}
	return b, nil
}

// DecodeReply parses a reply.
func DecodeReply(b []byte) (Reply, error) {
	var r Reply
	if len(b) < PacketHeaderLength || b[0] != ProtocolVersion {
		return r, ErrPacket
	}
	be := binary.BigEndian
	r.Operation, r.RequestID = Operation(b[1]), be.Uint32(b[2:])
	short := func(n int) bool { return len(b) < n }
	switch r.Operation {
	case OperationListManifests | ReplyFlag:
		if len(b) != listReplyLength {
			return r, ErrPacket
		}
		copy(r.Address[:], b[6:38])
		r.Total = be.Uint32(b[38:])
	case OperationGetManifest | ReplyFlag:
		if short(manifestReplyFixed) {
			return r, ErrPacket
		}
		copy(r.Address[:], b[6:38])
		r.Offset, r.Total, r.Payload = uint64(be.Uint32(b[38:])), be.Uint32(b[42:]), b[manifestReplyFixed:]
	case OperationGetBlockList | ReplyFlag:
		if short(blockListReplyFixed) {
			return r, ErrPacket
		}
		copy(r.Address[:], b[6:38])
		r.Offset, r.Length, r.Payload = uint64(be.Uint32(b[38:])), be.Uint16(b[42:]), b[blockListReplyFixed:]
		if len(r.Payload) != int(r.Length)*BlockEntryLength {
			return r, ErrPacket
		}
	case OperationGetBlocks | ReplyFlag:
		if short(blocksReplyFixed) {
			return r, ErrPacket
		}
		copy(r.Address[:], b[6:38])
		r.Offset, r.Length, r.Payload = be.Uint64(b[38:]), be.Uint16(b[46:]), b[blocksReplyFixed:]
		if len(r.Payload) != int(r.Length) {
			return r, ErrPacket
		}
	case OperationError:
		if short(errorReplyFixed) || len(b) != errorReplyFixed+int(b[7]) {
			return r, ErrPacket
		}
		r.ErrorCode, r.ErrorText = ErrorCode(b[6]), string(b[errorReplyFixed:])
	default:
		return r, fmt.Errorf("%w: operation %#x", ErrPacket, b[1])
	}
	return r, nil
}

// ReplyError is an error reply turned into a Go error.
type ReplyError struct {
	Code ErrorCode
	Text string
}

func (e *ReplyError) Error() string { return fmt.Sprintf("server error %d: %s", e.Code, e.Text) }
