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

// subscribeKind marks an app that runs a super programnetwork node on the
// phone. It subscribes to nodes, reads every have from its have queue, and
// requests each program, until the wanted one is held (#42). Its args are
// the comma separated nodes to subscribe to, the comma separated trusted
// signer keys in hex and the name in the wanted program's cxapp.txt, see "Announce and
// request" in docs/ProgramDistribution.md.
const subscribeKind = "subscribe"

// subscribeWait is how long the app waits for the program.
const subscribeWait = 30 * time.Second

// subscribeProgram runs a node with an empty store in the files dir,
// subscribes to the nodes the app names, prints every have it reads, and
// returns the program once the node holds it, verified, reporting each step
// to out. The node accepts any valid signer, and confirm asks the user
// about one that is not trusted.
func subscribeProgram(m *manifest.Manifest, out io.Writer, confirm func(cipher.PubKey) bool) (*manifest.Manifest, map[string][]byte, error) {
	if len(m.Args) != 3 {
		return nil, nil, fmt.Errorf("a subscribe app takes the nodes, the signer key and the program name as args")
	}
	upstream, name := strings.Split(m.Args[0], ","), m.Args[2]
	trusted, err := parseKeys(m.Args[1])
	if err != nil {
		return nil, nil, err
	}
	dir, err := app.DataDir()
	if err != nil {
		return nil, nil, err
	}
	storeDir := filepath.Join(dir, "subscribe-node")
	// The store starts empty, so every run requests the program.
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
	node, err := programnetwork.NewNode(conn, st, nil)
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
	fmt.Fprintf(out, "node on %s, subscribing to %s\n", conn.LocalAddr(), strings.Join(upstream, " "))
	for _, u := range upstream {
		if err := node.Subscribe(u); err != nil {
			return nil, nil, err
		}
	}
	timeout := time.After(subscribeWait)
	for {
		select {
		case <-node.Haves().Ready():
			node.Update(0)
			for q, ok := node.Haves().ReadyNext(); ok; q, ok = node.Haves().ReadyNext() {
				fmt.Fprintf(out, "read have %s from %s\n", q.Address.String()[:16], q.From)
			}
		case h := <-held:
			if h.Name != name {
				fmt.Fprintf(out, "received %s, waiting for %s\n", h.Name, name)
				continue
			}
			fmt.Fprintf(out, "requested %s from %s, held in %d ms\n", name, h.From, time.Since(start).Milliseconds())
			fmt.Fprintf(out, "manifest %s\n", h.Address)
			if err := reportSigner(out, h.Manifest.SignerPublicKey, trusted, confirm); err != nil {
				return nil, nil, err
			}
			fmt.Fprintf(out, "%d files, %d blocks, %d bytes, every block verified\n", len(h.Manifest.Files), len(h.Manifest.Blocks), h.Manifest.PackLength)
			// Let the have from the second node arrive, so the count shows
			// every have was read and the program requested once.
			deadline := time.After(time.Second)
			for waiting := true; waiting; {
				select {
				case <-node.Haves().Ready():
					node.Update(0)
					for q, ok := node.Haves().ReadyNext(); ok; q, ok = node.Haves().ReadyNext() {
						fmt.Fprintf(out, "read have %s from %s\n", q.Address.String()[:16], q.From)
					}
				case <-deadline:
					waiting = false
				}
			}
			s := node.Stats()
			fmt.Fprintf(out, "haves received %d, read %d, requests %d, downloads %d\n", s.HavesReceived, s.HavesRead, s.Requests, s.Downloads)
			program, err := programblocks.AppManifest(h.Files)
			if err != nil {
				return nil, nil, err
			}
			return program, h.Files, nil
		case <-timeout:
			s := node.Stats()
			return nil, nil, fmt.Errorf("%s did not arrive in %v: %d haves read, %d requests, %d failed", name, subscribeWait, s.HavesRead, s.Requests, s.DownloadFailures)
		}
	}
}
