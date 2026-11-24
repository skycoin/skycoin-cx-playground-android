package main

import (
	"fmt"
	"io"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
)

// thinKind marks an app that gets its program as a thin node (#44): it
// connects to one node, downloads the program, verifies it and is done,
// with no subscription and no state. Its args are the node, the trusted
// signer key in hex and the manifest address in hex, see "Node types (#34,
// #44)" in docs/ProgramDistribution.md.
const thinKind = "thin"

// thinProgram gets the program a thin app names, reporting each step to
// out.
func thinProgram(m *manifest.Manifest, out io.Writer) (*manifest.Manifest, map[string][]byte, error) {
	if len(m.Args) != 3 {
		return nil, nil, fmt.Errorf("a thin app takes the node, the signer key and the manifest address as args")
	}
	node, signerHex, address := m.Args[0], m.Args[1], m.Args[2]
	signer, err := cipher.PubKeyFromHex(signerHex)
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(out, "thin get %s from %s\n", address[:min(16, len(address))], node)
	p, stats, err := programnetwork.Get(node, address, programnetwork.GetOptions{Trusted: []cipher.PubKey{signer}})
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(out, "got %s from %s in %d ms\n", p.Name, p.From, p.Elapsed.Milliseconds())
	fmt.Fprintf(out, "signer %s verified\n", p.Manifest.SignerPublicKey.Hex())
	fmt.Fprintf(out, "%d files, %d blocks, %d bytes, every block verified\n", len(p.Manifest.Files), len(p.Manifest.Blocks), p.Manifest.PackLength)
	fmt.Fprintf(out, "%d requests, no subscription, nothing kept\n", stats.RequestsSent)
	program, err := programblocks.AppManifest(p.Files)
	if err != nil {
		return nil, nil, err
	}
	return program, p.Files, nil
}
