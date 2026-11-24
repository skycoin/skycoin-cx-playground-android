package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0magnet/gio/app"
	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// thinKind marks an app that runs a thin programnetwork node on the phone:
// it hears the haves of the nodes it subscribes to, stores nothing for
// others, and fetches the program it wants on demand. Its args are as for
// a flood app, see "Node types (#34)" in docs/ProgramDistribution.md.
const thinKind = "thin"

// thinWait is how long the app waits for the program to be heard of and
// fetched.
const thinWait = 30 * time.Second

// thinProgram runs a thin node with an empty store in the files dir,
// subscribes to the nodes the app names, and fetches each program it hears
// of until one has the wanted name, reporting each step to out.
func thinProgram(m *manifest.Manifest, out io.Writer) (*manifest.Manifest, map[string][]byte, error) {
	if len(m.Args) != 3 {
		return nil, nil, fmt.Errorf("a thin app takes the nodes, the signer key and the program name as args")
	}
	upstream, signerHex, name := strings.Split(m.Args[0], ","), m.Args[1], m.Args[2]
	signer, err := cipher.PubKeyFromHex(signerHex)
	if err != nil {
		return nil, nil, err
	}
	dir, err := app.DataDir()
	if err != nil {
		return nil, nil, err
	}
	storeDir := filepath.Join(dir, "thin-node")
	// The store starts empty, so every run fetches the program.
	if err := os.RemoveAll(storeDir); err != nil {
		return nil, nil, err
	}
	st, err := programstore.Open(storeDir)
	if err != nil {
		return nil, nil, err
	}
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return nil, nil, err
	}
	node, err := programnetwork.NewTypedNode(conn, st, []cipher.PubKey{signer}, programnetwork.Settings{Type: programnetwork.TypeThin})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	defer node.Close()
	go node.Run()
	start := time.Now()
	fmt.Fprintf(out, "thin node on %s, subscribing to %s\n", conn.LocalAddr(), strings.Join(upstream, " "))
	for _, u := range upstream {
		if err := node.Subscribe(u); err != nil {
			return nil, nil, err
		}
	}
	tried := map[programblocks.Hash]bool{}
	deadline := time.Now().Add(thinWait)
	for time.Now().Before(deadline) {
		for _, h := range node.Known() {
			if tried[h.Address] {
				continue
			}
			tried[h.Address] = true
			fmt.Fprintf(out, "heard of %s, %d bytes, fetching on demand\n", h.Address.String()[:16], h.ManifestLength+uint32(h.PackLength))
			p, err := node.Fetch(h.Address)
			if err != nil {
				fmt.Fprintf(out, "fetch: %v\n", err)
				delete(tried, h.Address)
				continue
			}
			if p.Name != name {
				fmt.Fprintf(out, "fetched %s, waiting for %s\n", p.Name, name)
				continue
			}
			s := node.Stats()
			fmt.Fprintf(out, "fetched %s on demand from %s in %d ms\n", name, p.From, time.Since(start).Milliseconds())
			fmt.Fprintf(out, "signer %s verified\n", p.Manifest.SignerPublicKey.Hex())
			fmt.Fprintf(out, "%d files, %d blocks, %d bytes, every block verified\n", len(p.Manifest.Files), len(p.Manifest.Blocks), p.Manifest.PackLength)
			fmt.Fprintf(out, "thin node holds %d for others, downloads %d, haves ignored %d, subscribers %d\n", len(heldBy(node, node.Known())), s.Downloads, s.HavesIgnored, s.Subscribers)
			program, err := programblocks.AppManifest(p.Files)
			if err != nil {
				return nil, nil, err
			}
			return program, p.Files, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	s := node.Stats()
	return nil, nil, fmt.Errorf("%s was not fetched in %v: %d haves, %d downloads", name, thinWait, s.HavesReceived, s.Downloads)
}

// heldBy returns the programs of haves that node holds for others.
func heldBy(node *programnetwork.Node, haves []programnetwork.Have) []programnetwork.Have {
	var out []programnetwork.Have
	for _, h := range haves {
		if node.Holds(h.Address) {
			out = append(out, h)
		}
	}
	return out
}
