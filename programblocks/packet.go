package programblocks

import (
	"bytes"
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
	// MinPacketSize and MaxPacketSize bound the packet size setting. At 16
	// bytes only some operations fit, see "Packet size sweep".
	MinPacketSize = 16
	MaxPacketSize = 65507
	// MinFullFormPacketSize is the smallest packet whose get blocks reply
	// holds a whole 512-byte block. Below it the full forms fail with error 6.
	MinFullFormPacketSize = blocksReplyFixed + DefaultBlockSize
	// MinPrefixLength and DefaultPrefixLength are the byte lengths of a
	// manifest address or block hash prefix in the short forms (D36).
	MinPrefixLength     = 4
	DefaultPrefixLength = 8
)

// Operation is the operation code of a packet.
type Operation byte

// The operations. A reply carries the request's code plus ReplyFlag, or
// OperationError. 0x05 to 0x08 are the short forms of #31, which name a
// manifest or a block by a hash prefix and read a part of one unit.
const (
	OperationListManifests       Operation = 0x01
	OperationGetManifest         Operation = 0x02
	OperationGetBlockList        Operation = 0x03
	OperationGetBlocks           Operation = 0x04
	OperationListManifestsPrefix Operation = 0x05
	OperationGetManifestByPrefix Operation = 0x06
	OperationGetBlockListBytes   Operation = 0x07
	OperationGetBlockPart        Operation = 0x08
	OperationError               Operation = 0xFF
)

// ErrorCode is the code of an error reply.
type ErrorCode byte

// The error codes.
const (
	ErrorCodeUnknownManifest    ErrorCode = 1
	ErrorCodeRangeOutside       ErrorCode = 2
	ErrorCodeMalformedRequest   ErrorCode = 3
	ErrorCodeUnsupportedVersion ErrorCode = 4
	ErrorCodeAmbiguousPrefix    ErrorCode = 5
	ErrorCodePacketTooSmall     ErrorCode = 6
)

// The packet lengths. A Fixed length excludes the payload, and the short
// forms add the prefix lengths to theirs.
const (
	listReplyLength        = PacketHeaderLength + 32 + 4
	manifestReplyFixed     = PacketHeaderLength + 32 + 4 + 4
	blockListReplyFixed    = PacketHeaderLength + 32 + 4 + 2
	blocksReplyFixed       = PacketHeaderLength + 32 + 8 + 2
	errorReplyFixed        = PacketHeaderLength + 2
	addressedRequestLength = PacketHeaderLength + 32 + 4 + 2
	getBlocksRequestLength = PacketHeaderLength + 32 + 8 + 2
	listManifestsReqLength = PacketHeaderLength

	listPrefixRequestLength = PacketHeaderLength + 1
	listPrefixReplyFixed    = PacketHeaderLength + 1 + 4
	byPrefixRequestFixed    = PacketHeaderLength + 1 + 4 + 2
	byPrefixReplyFixed      = PacketHeaderLength + 1 + 4 + 4
	blockPartRequestFixed   = PacketHeaderLength + 1 + 1 + 2 + 2
	blockPartReplyFixed     = PacketHeaderLength + 1 + 2 + 2
)

// MaxManifestPayload is the most manifest bytes one reply of packetSize holds.
func MaxManifestPayload(packetSize int) int { return packetSize - manifestReplyFixed }

// MaxBlockListEntries is the most block list entries one reply holds.
func MaxBlockListEntries(packetSize int) int {
	return max(0, (packetSize-blockListReplyFixed)/BlockEntryLength)
}

// MaxBlocksPayload is the most linear packing bytes one reply holds.
func MaxBlocksPayload(packetSize int) int { return packetSize - blocksReplyFixed }

// MaxByPrefixPayload is the most bytes one get manifest by prefix or get
// block list bytes reply holds with a prefix of prefixLength bytes.
func MaxByPrefixPayload(packetSize, prefixLength int) int {
	return packetSize - byPrefixReplyFixed - prefixLength
}

// MaxBlockPartPayload is the most block bytes one get block part reply
// holds with a block hash prefix of blockPrefixLength bytes.
func MaxBlockPartPayload(packetSize, blockPrefixLength int) int {
	return packetSize - blockPartReplyFixed - blockPrefixLength
}

// CheckPacketSize returns an error for a packet size outside MinPacketSize
// to MaxPacketSize. Whether an operation fits is checked per request.
func CheckPacketSize(packetSize int) error {
	if packetSize < MinPacketSize || packetSize > MaxPacketSize {
		return fmt.Errorf("packet size %d out of range %d to %d", packetSize, MinPacketSize, MaxPacketSize)
	}
	return nil
}

// ValidPrefixLength reports whether n bytes is a legal prefix length. Zero
// is legal only where the manifest may be any manifest.
func ValidPrefixLength(n int, zeroAllowed bool) bool {
	return (zeroAllowed && n == 0) || (n >= MinPrefixLength && n <= len(Hash{}))
}

// HasPrefix reports whether the first n bytes of h and prefix agree.
func (h Hash) HasPrefix(prefix Hash, n int) bool { return bytes.Equal(h[:n], prefix[:n]) }

// Request is a decoded request. Offset is the manifest offset, the first
// block ID, the linear packing offset, the block list byte offset or the
// offset in a block, and Length the byte length or the entry count, as the
// operation defines. The short forms carry only the first PrefixLength
// bytes of Address and BlockPrefixLength bytes of BlockHash. PrefixLength
// 0 in a get block part means any manifest.
type Request struct {
	Operation         Operation
	RequestID         uint32
	Address           Hash
	PrefixLength      int
	BlockHash         Hash
	BlockPrefixLength int
	Offset            uint64
	Length            uint16
}

// Reply is a decoded reply. Total is the manifest length for list manifests
// and get manifest, the block list length for get block list bytes and the
// block length for get block part. Length is the entry count for get block
// list. The short forms echo the prefixes of their request.
type Reply struct {
	Operation         Operation
	RequestID         uint32
	Address           Hash
	PrefixLength      int
	BlockHash         Hash
	BlockPrefixLength int
	Offset            uint64
	Total             uint32
	Length            uint16
	Payload           []byte
	ErrorCode         ErrorCode
	ErrorText         string
}

// ErrPacket marks a packet that cannot be decoded.
var ErrPacket = errors.New("malformed packet")

func putHeader(b []byte, op Operation, id uint32) {
	b[0] = ProtocolVersion
	b[1] = byte(op)
	binary.BigEndian.PutUint32(b[2:], id)
}

func appendPrefix(b []byte, h Hash, n int) []byte {
	return append(append(b, byte(n)), h[:n]...)
}

// readPrefix reads the prefix at b[at:] and returns it and the offset after it.
func readPrefix(b []byte, at int, zeroAllowed bool) (Hash, int, int, error) {
	var h Hash
	if len(b) <= at {
		return h, 0, 0, ErrPacket
	}
	n := int(b[at])
	if !ValidPrefixLength(n, zeroAllowed) || len(b) < at+1+n {
		return h, 0, 0, fmt.Errorf("%w: prefix of %d bytes", ErrPacket, n)
	}
	copy(h[:], b[at+1:at+1+n])
	return h, n, at + 1 + n, nil
}

// RequestLength returns the encoded length of r without encoding it.
func RequestLength(r Request) int {
	switch r.Operation {
	case OperationListManifests:
		return listManifestsReqLength
	case OperationGetBlocks:
		return getBlocksRequestLength
	case OperationListManifestsPrefix:
		return listPrefixRequestLength
	case OperationGetManifestByPrefix, OperationGetBlockListBytes:
		return byPrefixRequestFixed + r.PrefixLength
	case OperationGetBlockPart:
		return blockPartRequestFixed + r.PrefixLength + r.BlockPrefixLength
	default:
		return addressedRequestLength
	}
}

// EncodeRequest returns the bytes of r.
func EncodeRequest(r Request) []byte {
	be := binary.BigEndian
	b := make([]byte, PacketHeaderLength, RequestLength(r))
	putHeader(b, r.Operation, r.RequestID)
	switch r.Operation {
	case OperationListManifests:
	case OperationGetBlocks:
		b = append(b, r.Address[:]...)
		b = be.AppendUint64(b, r.Offset)
		b = be.AppendUint16(b, r.Length)
	case OperationListManifestsPrefix:
		b = append(b, byte(r.PrefixLength))
	case OperationGetManifestByPrefix, OperationGetBlockListBytes:
		b = appendPrefix(b, r.Address, r.PrefixLength)
		b = be.AppendUint32(b, uint32(r.Offset))
		b = be.AppendUint16(b, r.Length)
	case OperationGetBlockPart:
		b = appendPrefix(b, r.Address, r.PrefixLength)
		b = appendPrefix(b, r.BlockHash, r.BlockPrefixLength)
		b = be.AppendUint16(b, uint16(r.Offset))
		b = be.AppendUint16(b, r.Length)
	default:
		b = append(b, r.Address[:]...)
		b = be.AppendUint32(b, uint32(r.Offset))
		b = be.AppendUint16(b, r.Length)
	}
	return b
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
	bad := fmt.Errorf("%w: operation %#x with %d bytes", ErrPacket, byte(r.Operation), len(b))
	var err error
	switch r.Operation {
	case OperationListManifests:
		if len(b) != listManifestsReqLength {
			return r, bad
		}
	case OperationGetManifest, OperationGetBlockList:
		if len(b) != addressedRequestLength {
			return r, bad
		}
		copy(r.Address[:], b[6:38])
		r.PrefixLength, r.Offset, r.Length = len(Hash{}), uint64(be.Uint32(b[38:])), be.Uint16(b[42:])
	case OperationGetBlocks:
		if len(b) != getBlocksRequestLength {
			return r, bad
		}
		copy(r.Address[:], b[6:38])
		r.PrefixLength, r.Offset, r.Length = len(Hash{}), be.Uint64(b[38:]), be.Uint16(b[46:])
	case OperationListManifestsPrefix:
		if len(b) != listPrefixRequestLength || !ValidPrefixLength(int(b[6]), false) {
			return r, bad
		}
		r.PrefixLength = int(b[6])
	case OperationGetManifestByPrefix, OperationGetBlockListBytes:
		var at int
		if r.Address, r.PrefixLength, at, err = readPrefix(b, 6, false); err != nil || len(b) != at+6 {
			return r, bad
		}
		r.Offset, r.Length = uint64(be.Uint32(b[at:])), be.Uint16(b[at+4:])
	case OperationGetBlockPart:
		var at int
		if r.Address, r.PrefixLength, at, err = readPrefix(b, 6, true); err != nil {
			return r, bad
		}
		if r.BlockHash, r.BlockPrefixLength, at, err = readPrefix(b, at, false); err != nil || len(b) != at+4 {
			return r, bad
		}
		r.Offset, r.Length = uint64(be.Uint16(b[at:])), be.Uint16(b[at+2:])
	default:
		return r, bad
	}
	return r, nil
}

// EncodeReply returns the bytes of r, or an error when they would exceed
// packetSize. An error reply's text is cut to fit.
func EncodeReply(r Reply, packetSize int) ([]byte, error) {
	be := binary.BigEndian
	b := make([]byte, PacketHeaderLength, PacketHeaderLength+64+len(r.Payload))
	putHeader(b, r.Operation, r.RequestID)
	switch r.Operation {
	case OperationListManifests | ReplyFlag:
		b = append(b, r.Address[:]...)
		b = be.AppendUint32(b, r.Total)
	case OperationGetManifest | ReplyFlag:
		b = append(b, r.Address[:]...)
		b = be.AppendUint32(b, uint32(r.Offset))
		b = be.AppendUint32(b, r.Total)
		b = append(b, r.Payload...)
	case OperationGetBlockList | ReplyFlag:
		if len(r.Payload) != int(r.Length)*BlockEntryLength {
			return nil, fmt.Errorf("block list reply of %d entries with %d bytes", r.Length, len(r.Payload))
		}
		b = append(b, r.Address[:]...)
		b = be.AppendUint32(b, uint32(r.Offset))
		b = be.AppendUint16(b, r.Length)
		b = append(b, r.Payload...)
	case OperationGetBlocks | ReplyFlag:
		b = append(b, r.Address[:]...)
		b = be.AppendUint64(b, r.Offset)
		b = be.AppendUint16(b, uint16(len(r.Payload)))
		b = append(b, r.Payload...)
	case OperationListManifestsPrefix | ReplyFlag:
		b = appendPrefix(b, r.Address, r.PrefixLength)
		b = be.AppendUint32(b, r.Total)
	case OperationGetManifestByPrefix | ReplyFlag, OperationGetBlockListBytes | ReplyFlag:
		b = appendPrefix(b, r.Address, r.PrefixLength)
		b = be.AppendUint32(b, uint32(r.Offset))
		b = be.AppendUint32(b, r.Total)
		b = append(b, r.Payload...)
	case OperationGetBlockPart | ReplyFlag:
		b = appendPrefix(b, r.BlockHash, r.BlockPrefixLength)
		b = be.AppendUint16(b, uint16(r.Offset))
		b = be.AppendUint16(b, uint16(r.Total))
		b = append(b, r.Payload...)
	case OperationError:
		text := r.ErrorText[:max(0, min(len(r.ErrorText), MaxErrorTextLength, packetSize-errorReplyFixed))]
		b = append(b, byte(r.ErrorCode), byte(len(text)))
		b = append(b, text...)
	default:
		return nil, fmt.Errorf("no reply operation %#x", byte(r.Operation))
	}
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
	var at int
	var err error
	switch r.Operation {
	case OperationListManifests | ReplyFlag:
		if len(b) != listReplyLength {
			return r, ErrPacket
		}
		copy(r.Address[:], b[6:38])
		r.PrefixLength, r.Total = len(Hash{}), be.Uint32(b[38:])
	case OperationGetManifest | ReplyFlag:
		if short(manifestReplyFixed) {
			return r, ErrPacket
		}
		copy(r.Address[:], b[6:38])
		r.PrefixLength, r.Offset, r.Total, r.Payload = len(Hash{}), uint64(be.Uint32(b[38:])), be.Uint32(b[42:]), b[manifestReplyFixed:]
	case OperationGetBlockList | ReplyFlag:
		if short(blockListReplyFixed) {
			return r, ErrPacket
		}
		copy(r.Address[:], b[6:38])
		r.PrefixLength, r.Offset, r.Length, r.Payload = len(Hash{}), uint64(be.Uint32(b[38:])), be.Uint16(b[42:]), b[blockListReplyFixed:]
		if len(r.Payload) != int(r.Length)*BlockEntryLength {
			return r, ErrPacket
		}
	case OperationGetBlocks | ReplyFlag:
		if short(blocksReplyFixed) {
			return r, ErrPacket
		}
		copy(r.Address[:], b[6:38])
		r.PrefixLength, r.Offset, r.Length, r.Payload = len(Hash{}), be.Uint64(b[38:]), be.Uint16(b[46:]), b[blocksReplyFixed:]
		if len(r.Payload) != int(r.Length) {
			return r, ErrPacket
		}
	case OperationListManifestsPrefix | ReplyFlag:
		if r.Address, r.PrefixLength, at, err = readPrefix(b, 6, false); err != nil || len(b) != at+4 {
			return r, ErrPacket
		}
		r.Total = be.Uint32(b[at:])
	case OperationGetManifestByPrefix | ReplyFlag, OperationGetBlockListBytes | ReplyFlag:
		if r.Address, r.PrefixLength, at, err = readPrefix(b, 6, false); err != nil || short(at+8) {
			return r, ErrPacket
		}
		r.Offset, r.Total, r.Payload = uint64(be.Uint32(b[at:])), be.Uint32(b[at+4:]), b[at+8:]
	case OperationGetBlockPart | ReplyFlag:
		if r.BlockHash, r.BlockPrefixLength, at, err = readPrefix(b, 6, false); err != nil || short(at+4) {
			return r, ErrPacket
		}
		r.Offset, r.Total, r.Payload = uint64(be.Uint16(b[at:])), uint32(be.Uint16(b[at+2:])), b[at+4:]
		r.Length = uint16(len(r.Payload))
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

// Is lets errors.Is match a packet too small error reply to
// ErrPacketTooSmall.
func (e *ReplyError) Is(target error) bool {
	return target == ErrPacketTooSmall && e.Code == ErrorCodePacketTooSmall
}
