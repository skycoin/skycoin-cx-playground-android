// Package programnetwork is the DHT prototype of #18: nodes that subscribe
// to each other, announce the programs they hold with have packets, put
// every have they receive in a have queue, request only what their policy
// keeps (#42), and exchange public peers as Bitcoin does (#41). A thin
// node is Get, a one-shot fetch (#44). See "DHT prototype (#18)" in
// docs/ProgramDistribution.md.
package programnetwork

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// The operations this package adds to those of programblocks. A reply adds
// programblocks.ReplyFlag, and an error reply is programblocks.OperationError.
// Get peers is Bitcoin's getaddr, and its reply is addr in the shape of
// BIP155 addrv2 (#41).
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
	getPeersRequestLength  = programblocks.PacketHeaderLength
	getPeersReplyFixed     = programblocks.PacketHeaderLength + 1
)

// The lengths of a peer entry: last seen u32, flags u8, network id u8,
// address length u8, the address, port u16.
const (
	peerEntryFixed = 4 + 1 + 1 + 1 + 2
	// MaxPeerEntryLength is the length of an IPv6 entry, the longest.
	MaxPeerEntryLength = peerEntryFixed + 16
)

// Have announces that the sender holds the program whose manifest address
// is Address, with a manifest of ManifestLength bytes and a linear packing
// of PackLength bytes.
type Have struct {
	Address        programblocks.Hash
	ManifestLength uint32
	PackLength     uint64
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

// EncodeGetPeers asks for shareable peers, as Bitcoin's getaddr.
func EncodeGetPeers(id uint32) []byte {
	return header(OperationGetPeers, id, getPeersRequestLength)
}

// DecodeGetPeers checks a get peers request.
func DecodeGetPeers(b []byte) error {
	if len(b) != getPeersRequestLength {
		return malformed(OperationGetPeers, len(b))
	}
	return nil
}

// EncodeGetPeersReply returns a get peers reply with as many of entries as
// fit in packetSize, at most MaxPeersPerReply, and the number it holds.
func EncodeGetPeersReply(id uint32, entries []PeerEntry, packetSize int) ([]byte, int) {
	b := header(OperationGetPeers|programblocks.ReplyFlag, id, packetSize)
	b = append(b, 0)
	n := 0
	for _, e := range entries {
		eb := EncodePeerEntry(e)
		if len(b)+len(eb) > packetSize || n == MaxPeersPerReply {
			break
		}
		b = append(b, eb...)
		n++
	}
	b[getPeersReplyFixed-1] = byte(n)
	return b, n
}

// DecodeGetPeersReply parses a get peers reply and returns its entries.
func DecodeGetPeersReply(b []byte) ([]PeerEntry, error) {
	bad := malformed(OperationGetPeers|programblocks.ReplyFlag, len(b))
	if len(b) < getPeersReplyFixed || int(b[6]) > MaxPeersPerReply {
		return nil, bad
	}
	var out []PeerEntry
	rest := b[getPeersReplyFixed:]
	for range int(b[6]) {
		e, n, err := DecodePeerEntry(rest)
		if err != nil {
			return nil, err
		}
		out, rest = append(out, e), rest[n:]
	}
	if len(rest) != 0 {
		return nil, bad
	}
	return out, nil
}

// EncodePeerEntry returns the bytes of one peer entry: last seen in Unix
// seconds (u32), the flag byte, the BIP155 network id, the address length
// and bytes, and the port (u16).
func EncodePeerEntry(e PeerEntry) []byte {
	addr := e.Address.Addr().Unmap()
	raw := addr.AsSlice()
	b := binary.BigEndian.AppendUint32(make([]byte, 0, peerEntryFixed+len(raw)), uint32(e.LastSeen.Unix()))
	b = append(b, e.Flags, e.Network(), byte(len(raw)))
	b = append(b, raw...)
	return binary.BigEndian.AppendUint16(b, e.Address.Port())
}

// DecodePeerEntry parses the peer entry at the start of b and returns it
// with its length. An unknown network id or an address of the wrong length
// is malformed.
func DecodePeerEntry(b []byte) (PeerEntry, int, error) {
	bad := fmt.Errorf("%w: peer entry in %d bytes", programblocks.ErrPacket, len(b))
	if len(b) < peerEntryFixed {
		return PeerEntry{}, 0, bad
	}
	network, n := b[5], int(b[6])
	if (network != NetworkIPv4 || n != 4) && (network != NetworkIPv6 || n != 16) || len(b) < peerEntryFixed+n {
		return PeerEntry{}, 0, bad
	}
	addr, _ := netip.AddrFromSlice(b[7 : 7+n])
	port := binary.BigEndian.Uint16(b[7+n:])
	e := PeerEntry{Address: netip.AddrPortFrom(addr, port), LastSeen: time.Unix(int64(binary.BigEndian.Uint32(b)), 0).UTC(), Flags: b[4]}
	return e, peerEntryFixed + n, nil
}
