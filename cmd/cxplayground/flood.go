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

// floodKind marks an app that runs a programnetwork node on the phone and
// waits for a program to arrive by flood. Its args are the comma separated
// nodes to subscribe to, the trusted signer key in hex and the name in the
// wanted program's cxapp.json, see "DHT prototype (#18)" in
// docs/ProgramDistribution.md.
const floodKind = "flood"

// floodWait is how long the app waits for the program to arrive.
const floodWait = 30 * time.Second

// floodProgram runs a node with an empty store in the files dir, subscribes
// to the nodes the app names, and returns the program once the node holds
// it, verified, reporting each step to out.
func floodProgram(m *manifest.Manifest, out io.Writer) (*manifest.Manifest, map[string][]byte, error) {
	if len(m.Args) != 3 {
		return nil, nil, fmt.Errorf("a flood app takes the nodes, the signer key and the program name as args")
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
	storeDir := filepath.Join(dir, "flood-node")
	// The store starts empty, so every run receives the program by flood.
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
	node, err := programnetwork.NewNode(conn, st, []cipher.PubKey{signer})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	defer node.Close()
	held := make(chan programnetwork.Held, 4)
	node.OnHeld = func(h programnetwork.Held) {
		select {
		case held <- h:
		default:
		}
	}
	go node.Run()
	start := time.Now()
	fmt.Fprintf(out, "flood node on %s, subscribing to %s\n", conn.LocalAddr(), strings.Join(upstream, " "))
	for _, u := range upstream {
		if err := node.Subscribe(u); err != nil {
			return nil, nil, err
		}
	}
	timeout := time.After(floodWait)
	for {
		select {
		case h := <-held:
			if h.Name != name {
				fmt.Fprintf(out, "received %s, waiting for %s\n", h.Name, name)
				continue
			}
			fmt.Fprintf(out, "received %s by flood from %s in %d ms\n", name, h.From, time.Since(start).Milliseconds())
			fmt.Fprintf(out, "manifest %s\n", h.Address)
			fmt.Fprintf(out, "signer %s verified\n", h.Manifest.SignerPublicKey.Hex())
			fmt.Fprintf(out, "%d files, %d blocks, %d bytes, every block verified\n", len(h.Manifest.Files), len(h.Manifest.Blocks), h.Manifest.PackLength)
			// Let the have from the second node arrive, so the count shows it
			// was not downloaded twice.
			time.Sleep(500 * time.Millisecond)
			s := node.Stats()
			fmt.Fprintf(out, "node downloads %d, haves received %d, duplicates %d\n", s.Downloads, s.HavesReceived, s.HavesDuplicate)
			program, err := programblocks.AppManifest(h.Files)
			if err != nil {
				return nil, nil, err
			}
			return program, h.Files, nil
		case <-timeout:
			s := node.Stats()
			return nil, nil, fmt.Errorf("%s did not arrive in %v: %d haves, %d downloads, %d failed", name, floodWait, s.HavesReceived, s.Downloads, s.DownloadFailures)
		}
	}
}
