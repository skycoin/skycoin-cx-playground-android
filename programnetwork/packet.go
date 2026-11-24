// Package programnetwork is the DHT prototype of #18: nodes that subscribe
// to each other, announce the programs they hold with have packets, flood
// new programs to every subscriber once, and share peers by class, group
// and address scope. See "DHT prototype (#18)" in
// docs/ProgramDistribution.md.
package programnetwork

import (
	"encoding/binary"
	"fmt"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// The operations this package adds to those of programblocks. A reply adds
// programblocks.ReplyFlag, and an error reply is programblocks.OperationError.
const (
	OperationHave      programblocks.Operation = 0x10
	OperationSubscribe programblocks.Operation = 0x11
	OperationGetPeers  programblocks.Operation = 0x12
)

// The packet lengths, header included.
const (
	haveRequestLength      = programblocks.PacketHeaderLength + 32 + 4 + 8
	haveReplyLength        = programblocks.PacketHeaderLength + 32
	subscribeRequestLength = programblocks.PacketHeaderLength + 4
	subscribeReplyLength   = programblocks.PacketHeaderLength + 4
	getPeersRequestLength  = programblocks.PacketHeaderLength + 2
	getPeersReplyFixed     = programblocks.PacketHeaderLength + 2 + 2 + 1
)

// The limits of a peer entry in a get peers reply.
const (
	MaxAddressLength   = 64
	MaxGroupNameLength = 32
	peerEntryFixed     = 1 + 1 + 1
	// MaxPeerEntryLength is the longest peer entry.
	MaxPeerEntryLength = peerEntryFixed + MaxAddressLength + MaxGroupNameLength
)

// Have announces that the sender holds the program whose manifest address
// is Address, with a manifest of ManifestLength bytes and a linear packing
// of PackLength bytes.
type Have struct {
	Address        programblocks.Hash
	ManifestLength uint32
	PackLength     uint64
}

// PeerEntry is one peer in a get peers reply.
type PeerEntry struct {
	Scope   AddressScope
	Address string
	Group   string
}

// GetPeersReply is one page of a peer's shareable peers. Total is the
// number of peers shared with the asker, First the index of Entries[0].
type GetPeersReply struct {
	Total   uint16
	First   uint16
	Entries []PeerEntry
}

func header(op programblocks.Operation, id uint32, length int) []byte {
	b := make([]byte, programblocks.PacketHeaderLength, length)
	b[0] = programblocks.ProtocolVersion
	b[1] = byte(op)
	binary.BigEndian.PutUint32(b[2:], id)
	return b
}

// PacketOperation returns the operation and request ID of a packet, and
// false when the packet is shorter than the header or of another version.
func PacketOperation(b []byte) (programblocks.Operation, uint32, bool) {
	if len(b) < programblocks.PacketHeaderLength || b[0] != programblocks.ProtocolVersion {
		return 0, 0, false
	}
	return programblocks.Operation(b[1]), binary.BigEndian.Uint32(b[2:]), true
}

func malformed(op programblocks.Operation, n int) error {
	return fmt.Errorf("%w: operation %#x with %d bytes", programblocks.ErrPacket, byte(op), n)
}

// EncodeHave returns a have request. The reply, EncodeHaveReply, echoes the
// address.
func EncodeHave(id uint32, h Have) []byte {
	b := header(OperationHave, id, haveRequestLength)
	b = append(b, h.Address[:]...)
	b = binary.BigEndian.AppendUint32(b, h.ManifestLength)
	return binary.BigEndian.AppendUint64(b, h.PackLength)
}

// DecodeHave parses a have request.
func DecodeHave(b []byte) (Have, error) {
	var h Have
	if len(b) != haveRequestLength {
		return h, malformed(OperationHave, len(b))
	}
	copy(h.Address[:], b[6:38])
	h.ManifestLength = binary.BigEndian.Uint32(b[38:])
	h.PackLength = binary.BigEndian.Uint64(b[42:])
	return h, nil
}

// EncodeHaveReply returns the acknowledgement of a have.
func EncodeHaveReply(id uint32, addr programblocks.Hash) []byte {
	return append(header(OperationHave|programblocks.ReplyFlag, id, haveReplyLength), addr[:]...)
}

// DecodeHaveReply parses the acknowledgement of a have.
func DecodeHaveReply(b []byte) (programblocks.Hash, error) {
	var addr programblocks.Hash
	if len(b) != haveReplyLength {
		return addr, malformed(OperationHave|programblocks.ReplyFlag, len(b))
	}
	copy(addr[:], b[6:])
	return addr, nil
}

// EncodeSubscribe asks for have packets for leaseSeconds. The reply,
// EncodeSubscribeReply, carries the lease granted.
func EncodeSubscribe(id uint32, leaseSeconds uint32) []byte {
	return binary.BigEndian.AppendUint32(header(OperationSubscribe, id, subscribeRequestLength), leaseSeconds)
}

// EncodeSubscribeReply returns the reply to a subscribe.
func EncodeSubscribeReply(id uint32, leaseSeconds uint32) []byte {
	return binary.BigEndian.AppendUint32(header(OperationSubscribe|programblocks.ReplyFlag, id, subscribeReplyLength), leaseSeconds)
}

// DecodeLease parses a subscribe request or reply and returns its lease in
// seconds.
func DecodeLease(b []byte) (uint32, error) {
	if len(b) != subscribeRequestLength {
		return 0, malformed(OperationSubscribe, len(b))
	}
	return binary.BigEndian.Uint32(b[6:]), nil
}

// EncodeGetPeers asks for the shareable peers from index first on.
func EncodeGetPeers(id uint32, first uint16) []byte {
	return binary.BigEndian.AppendUint16(header(OperationGetPeers, id, getPeersRequestLength), first)
}

// DecodeGetPeers parses a get peers request and returns its first index.
func DecodeGetPeers(b []byte) (uint16, error) {
	if len(b) != getPeersRequestLength {
		return 0, malformed(OperationGetPeers, len(b))
	}
	return binary.BigEndian.Uint16(b[6:]), nil
}

// EncodeGetPeersReply returns a get peers reply with as many of entries,
// starting at index first of total, as fit in packetSize, and the number
// of entries it holds. An entry with a field too long is an error.
func EncodeGetPeersReply(id uint32, total, first uint16, entries []PeerEntry, packetSize int) ([]byte, int, error) {
	b := header(OperationGetPeers|programblocks.ReplyFlag, id, packetSize)
	b = binary.BigEndian.AppendUint16(b, total)
	b = binary.BigEndian.AppendUint16(b, first)
	b = append(b, 0)
	n := 0
	for _, e := range entries {
		if len(e.Address) == 0 || len(e.Address) > MaxAddressLength || len(e.Group) > MaxGroupNameLength {
			return nil, 0, fmt.Errorf("peer entry %q in group %q is too long", e.Address, e.Group)
		}
		if len(b)+peerEntryFixed+len(e.Address)+len(e.Group) > packetSize || n == 255 {
			break
		}
		b = append(b, byte(e.Scope), byte(len(e.Address)))
		b = append(b, e.Address...)
		b = append(b, byte(len(e.Group)))
		b = append(b, e.Group...)
		n++
	}
	b[getPeersReplyFixed-1] = byte(n)
	return b, n, nil
}

// DecodeGetPeersReply parses a get peers reply.
func DecodeGetPeersReply(b []byte) (GetPeersReply, error) {
	var r GetPeersReply
	bad := malformed(OperationGetPeers|programblocks.ReplyFlag, len(b))
	if len(b) < getPeersReplyFixed {
		return r, bad
	}
	r.Total, r.First = binary.BigEndian.Uint16(b[6:]), binary.BigEndian.Uint16(b[8:])
	count := int(b[10])
	rest := b[getPeersReplyFixed:]
	for range count {
		if len(rest) < 2 {
			return r, bad
		}
		e := PeerEntry{Scope: AddressScope(rest[0])}
		n := int(rest[1])
		if len(rest) < 2+n+1 {
			return r, bad
		}
		e.Address = string(rest[2 : 2+n])
		rest = rest[2+n:]
		g := int(rest[0])
		if len(rest) < 1+g {
			return r, bad
		}
		e.Group, rest = string(rest[1:1+g]), rest[1+g:]
		r.Entries = append(r.Entries, e)
	}
	if len(rest) != 0 {
		return r, bad
	}
	return r, nil
}
