package programblocks

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/pierrec/lz4/v4"
	"github.com/skycoin/skycoin/src/cipher"
)

// The envelope of a manifest is four lines and the data (D30):
//
//	CXPM 2          the header, "CXPM 2 #lz" when the data is one LZ4 block
//	<signature>     130 hex digits, secp256k1 over the hash
//	<hash>          64 hex digits, Sum of the data bytes, the manifest address
//	<length>        the data bytes, decimal
//	<data>          the text form, or its LZ4 block
//
// The hash, signature and length are of the data bytes exactly as they
// follow, so a reader checks them before it decompresses or parses anything.
const (
	// CompressedMark ends the header line of an envelope whose data is LZ4.
	CompressedMark = "#lz"
	// MinEnvelopeLength is the shortest envelope, with no data.
	MinEnvelopeLength = len(Magic) + 3 + 2*SignatureLength + 1 + 2*len(Hash{}) + 1 + 2
	// MaxEnvelopeHeaderLength is the longest header, the four lines.
	MaxEnvelopeHeaderLength = len(Magic) + 3 + 1 + len(CompressedMark) + 2*SignatureLength + 1 + 2*len(Hash{}) + 1 + 8
)

// Header is the envelope before the data, enough to check the signature
// against the hash before the data arrives.
type Header struct {
	Compressed      bool
	Signature       cipher.Sig
	SignerPublicKey cipher.PubKey
	// DataLength is the length of the data after the header.
	DataLength uint64
	// Length is the length of the header itself.
	Length int
}

func envelopeHeaderLine(compressed bool) string {
	line := Magic + " " + strconv.Itoa(FormatVersion)
	if compressed {
		line += " " + CompressedMark
	}
	return line
}

// Data returns the data bytes of m's envelope: the text form, as one LZ4
// block when m.Compressed.
func (m *Manifest) Data() []byte {
	text := m.EncodeText()
	if !m.Compressed {
		return text
	}
	out := make([]byte, lz4.CompressBlockBound(len(text)))
	var c lz4.Compressor
	n, err := c.CompressBlock(text, out)
	if err != nil || n == 0 {
		panic(fmt.Sprintf("lz4 of %d bytes into %d: %v", len(text), len(out), err))
	}
	return out[:n]
}

// Sign sets the signature of m, over its address, and the signer key.
func (m *Manifest) Sign(secret cipher.SecKey) error {
	pub, err := cipher.PubKeyFromSecKey(secret)
	if err != nil {
		return err
	}
	sig, err := cipher.SignHash(cipher.SHA256(m.Address()), secret)
	if err != nil {
		return err
	}
	m.SignerPublicKey, m.Signature = pub, sig
	return nil
}

// Encode returns the envelope of m. Sign m first.
func (m *Manifest) Encode() []byte {
	data := m.Data()
	addr := Sum(data)
	b := make([]byte, 0, MaxEnvelopeHeaderLength+len(data))
	b = append(b, envelopeHeaderLine(m.Compressed)...)
	b = append(b, '\n')
	b = hex.AppendEncode(b, m.Signature[:])
	b = append(b, '\n')
	b = hex.AppendEncode(b, addr[:])
	b = append(b, '\n')
	b = strconv.AppendUint(b, uint64(len(data)), 10)
	b = append(b, '\n')
	return append(b, data...)
}

// HeaderLength returns the length of the envelope header at the start of
// b, ErrTruncated when b ends before its fourth line does.
func HeaderLength(b []byte) (int, error) {
	n := 0
	for lines := 0; lines < 4; lines++ {
		i := bytes.IndexByte(b[n:min(len(b), MaxEnvelopeHeaderLength)], '\n')
		if i < 0 {
			if len(b) < MaxEnvelopeHeaderLength {
				return 0, ErrTruncated
			}
			return 0, fmt.Errorf("%w: no envelope header in %d bytes", ErrMalformed, MaxEnvelopeHeaderLength)
		}
		n += i + 1
	}
	return n, nil
}

// ParseHeader reads the envelope header at the start of b and checks that
// the signature matches the hash. It returns the header and the hash, the
// manifest address. b may hold the whole envelope or the first packet.
func ParseHeader(b []byte) (Header, Hash, error) {
	var h Header
	var addr Hash
	n, err := HeaderLength(b)
	if err != nil {
		return h, addr, err
	}
	lines := bytes.SplitN(b[:n-1], []byte("\n"), 4)
	switch string(lines[0]) {
	case envelopeHeaderLine(false):
	case envelopeHeaderLine(true):
		h.Compressed = true
	default:
		return h, addr, fmt.Errorf("%w: envelope header %q, want %q", ErrMalformed, lines[0], envelopeHeaderLine(false))
	}
	if !lowerHex(lines[1], h.Signature[:]) || !lowerHex(lines[2], addr[:]) {
		return h, addr, fmt.Errorf("%w: envelope signature or hash is not lower case hex of %d and %d bytes", ErrMalformed, SignatureLength, len(addr))
	}
	length, err := strconv.ParseUint(string(lines[3]), 10, 64)
	if err != nil || length > MaxManifestLength || (len(lines[3]) > 1 && lines[3][0] == '0') {
		return h, addr, fmt.Errorf("%w: envelope length %q", ErrMalformed, lines[3])
	}
	h.DataLength, h.Length = length, n
	pub, err := cipher.PubKeyFromSig(h.Signature, cipher.SHA256(addr))
	if err == nil {
		err = cipher.VerifyPubKeySignedHash(pub, h.Signature, cipher.SHA256(addr))
	}
	if err != nil {
		return h, addr, fmt.Errorf("%w: %v", ErrSignature, err)
	}
	h.SignerPublicKey = pub
	return h, addr, nil
}

func lowerHex(s []byte, out []byte) bool {
	if len(s) != 2*len(out) || !bytes.Equal(bytes.ToLower(s), s) {
		return false
	}
	_, err := hex.Decode(out, s)
	return err == nil
}

// Decode parses and checks a whole envelope: the signature, the hash and
// length of the data, and then the text form with its sizes and Merkle
// hashes. want is the address it was asked for, or the zero Hash to accept
// any.
func Decode(b []byte, want Hash) (*Manifest, Hash, error) {
	h, addr, err := ParseHeader(b)
	if err != nil {
		return nil, addr, err
	}
	if want != (Hash{}) && addr != want {
		return nil, addr, fmt.Errorf("%w: got %s, want %s", ErrAddress, addr, want)
	}
	data := b[h.Length:]
	if uint64(len(data)) != h.DataLength {
		if uint64(len(data)) < h.DataLength {
			return nil, addr, ErrTruncated
		}
		return nil, addr, fmt.Errorf("%w: %d data bytes, envelope says %d", ErrMalformed, len(data), h.DataLength)
	}
	if Sum(data) != addr {
		return nil, addr, ErrDataHash
	}
	text := data
	if h.Compressed {
		if text, err = decompress(data); err != nil {
			return nil, addr, err
		}
	}
	m, err := DecodeText(text)
	if err != nil {
		return nil, addr, err
	}
	m.Compressed, m.Signature, m.SignerPublicKey = h.Compressed, h.Signature, h.SignerPublicKey
	return m, addr, nil
}

// decompress returns the text of an LZ4 block of at most MaxManifestLength
// bytes.
func decompress(data []byte) ([]byte, error) {
	for size := min(4*len(data)+64, MaxManifestLength); ; size = min(2*size, MaxManifestLength) {
		out := make([]byte, size)
		n, err := lz4.UncompressBlock(data, out)
		if err == nil {
			return out[:n], nil
		}
		if !errors.Is(err, lz4.ErrInvalidSourceShortBuffer) || size == MaxManifestLength {
			return nil, fmt.Errorf("%w: lz4 data: %v", ErrMalformed, err)
		}
	}
}
