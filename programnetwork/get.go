package programnetwork

import (
	"encoding/hex"
	"fmt"
	"net"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// GetOptions are the settings of a thin get.
type GetOptions struct {
	Pinned     []cipher.PubKey // when set, only these signers are accepted; any valid signer otherwise
	PacketSize int             // programblocks.DefaultPacketSize when 0
	Forms      programblocks.Forms
	MaxTries   int // programblocks.DefaultMaxTries when 0
}

// Get is the thin node of #44 (D39), wget for the DHT. It connects to the
// node at node, downloads the program whose manifest address or address
// prefix is target in hex, verifies it and returns. It subscribes to
// nothing, receives no haves and keeps no state.
func Get(node, target string, o GetOptions) (Held, programblocks.ClientStats, error) {
	b, err := hex.DecodeString(target)
	if err != nil || len(b) < programblocks.MinPrefixLength || len(b) > len(programblocks.Hash{}) {
		return Held{}, programblocks.ClientStats{}, fmt.Errorf("address %q: want %d to %d bytes in hex", target, programblocks.MinPrefixLength, len(programblocks.Hash{}))
	}
	var addr programblocks.Hash
	copy(addr[:], b)
	start := time.Now()
	conn, err := net.Dial("udp", node)
	if err != nil {
		return Held{}, programblocks.ClientStats{}, err
	}
	defer conn.Close()
	c := programblocks.NewClient(conn, o.Pinned...)
	c.Forms = o.Forms
	if o.PacketSize != 0 {
		c.PacketSize = o.PacketSize
	}
	if o.MaxTries != 0 {
		c.MaxTries = o.MaxTries
	}
	var m *programblocks.Manifest
	if len(b) == len(addr) {
		m, _, err = c.FetchManifest(addr)
	} else {
		m, _, addr, err = c.FetchManifestByPrefix(addr, len(b))
	}
	if err != nil {
		return Held{}, c.Stats, err
	}
	files, err := c.FetchFiles(addr, m)
	if err != nil {
		return Held{}, c.Stats, err
	}
	return Held{Address: addr, Manifest: m, Files: files, Name: programName(files, addr), From: node, Elapsed: time.Since(start)}, c.Stats, nil
}
