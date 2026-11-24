// Command programnetwork runs a node of the DHT prototype of #18 and
// manages its peer list file, as described in "DHT prototype (#18)" in
// docs/ProgramDistribution.md.
//
//	programnetwork node [-addr :7010] [-store DIR] [-peers FILE] [-packet 1200]
//	                    [-signer PUBKEY,...] [-subscribe HOST:PORT,...]
//	                    [-publish DIR (-seed TEXT | -secret HEX)]
//	programnetwork getpeers HOST:PORT
//	programnetwork peers [-file peers.txt] list
//	programnetwork peers [-file peers.txt] add ADDRESS GROUP CLASS [COMMENT...]
//	programnetwork peers [-file peers.txt] remove ADDRESS
//	programnetwork peers [-file peers.txt] set-class ADDRESS CLASS
//	programnetwork peers [-file peers.txt] set-group ADDRESS GROUP
//	programnetwork peers [-file peers.txt] group-add NAME SCOPE OWNER OPERATOR SECURITY-GROUP
//	programnetwork peers [-file peers.txt] group-remove NAME
//	programnetwork peers [-file peers.txt] shared-with ADDRESS
//
// A node downloads only programs signed by a -signer key or its own
// publishing key. With -publish, every subdirectory of DIR is published as
// a program and flooded to the subscribers. Classes are private,
// own-infrastructure, lan-group, user-shareable and public; scopes are
// lan, internal and global.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "node":
		err = node(os.Args[2:])
	case "getpeers":
		err = getPeers(os.Args[2:])
	case "peers":
		err = peers(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "programnetwork:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: programnetwork node|getpeers|peers, see go doc ./cmd/programnetwork")
	os.Exit(2)
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func node(args []string) error {
	fl := flag.NewFlagSet("node", flag.ExitOnError)
	addr := fl.String("addr", ":7010", "UDP address to listen on")
	storeDir := fl.String("store", "programnetwork-store", "store directory of the programs this node holds")
	peersFile := fl.String("peers", "", "peer list file that get peers answers from")
	packet := fl.Int("packet", programblocks.DefaultPacketSize, "largest packet in bytes, 1200 by default, 16 to 65507")
	signers := fl.String("signer", "", "comma separated public keys, hex, whose programs the node downloads")
	subscribe := fl.String("subscribe", "", "comma separated nodes to subscribe to")
	publish := fl.String("publish", "", "directory of program directories to publish")
	seed := fl.String("seed", "", "derive the publishing key from this seed (tests only when public)")
	secretHex := fl.String("secret", "", "publishing secret key in hex")
	fl.Parse(args)
	var trusted []cipher.PubKey
	for _, s := range splitList(*signers) {
		k, err := cipher.PubKeyFromHex(s)
		if err != nil {
			return fmt.Errorf("-signer %s: %v", s, err)
		}
		trusted = append(trusted, k)
	}
	var secret cipher.SecKey
	var err error
	switch {
	case *secretHex != "":
		secret, err = cipher.SecKeyFromHex(*secretHex)
	case *seed != "":
		_, secret, err = cipher.GenerateDeterministicKeyPair([]byte(*seed))
	case *publish != "":
		err = fmt.Errorf("-publish needs -seed or -secret")
	}
	if err != nil {
		return err
	}
	if secret != (cipher.SecKey{}) {
		pk, err := cipher.PubKeyFromSecKey(secret)
		if err != nil {
			return err
		}
		trusted = append(trusted, pk)
	}
	if len(trusted) == 0 {
		return fmt.Errorf("give -signer, or -publish with its key")
	}
	st, err := programstore.Open(*storeDir)
	if err != nil {
		return err
	}
	conn, err := net.ListenPacket("udp", *addr)
	if err != nil {
		return err
	}
	n, err := programnetwork.NewNode(conn, st, trusted)
	if err != nil {
		return err
	}
	n.PacketSize = *packet
	n.Logf = func(format string, args ...any) {
		fmt.Printf("%s %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	}
	if *peersFile != "" {
		l, err := programnetwork.ReadPeerList(*peersFile)
		if err != nil {
			return err
		}
		n.SetPeers(l)
	}
	fmt.Printf("node listening on %s, packet %d bytes, %d trusted signers\n", conn.LocalAddr(), *packet, len(trusted))
	done := make(chan error, 1)
	go func() { done <- n.Run() }()
	for _, s := range splitList(*subscribe) {
		if err := n.Subscribe(s); err != nil {
			return fmt.Errorf("subscribe to %s: %v", s, err)
		}
	}
	if *publish != "" {
		entries, err := os.ReadDir(*publish)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			files, err := programblocks.ReadProgramDirectory(filepath.Join(*publish, e.Name()))
			if err != nil {
				return err
			}
			if _, err := n.Publish(files, secret, time.Now()); err != nil {
				return fmt.Errorf("publish %s: %v", e.Name(), err)
			}
		}
	}
	return <-done
}

func getPeers(args []string) error {
	if len(args) != 1 {
		usage()
	}
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "programnetwork-getpeers")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	st, err := programstore.Open(dir)
	if err != nil {
		return err
	}
	n, err := programnetwork.NewNode(conn, st, nil)
	if err != nil {
		return err
	}
	go n.Run()
	defer n.Close()
	entries, err := n.GetPeers(args[0])
	if err != nil {
		return err
	}
	for _, e := range entries {
		fmt.Printf("%s %s %s\n", e.Address, e.Group, e.Scope)
	}
	return nil
}

func peers(args []string) error {
	fl := flag.NewFlagSet("peers", flag.ExitOnError)
	file := fl.String("file", "peers.txt", "peer list file")
	fl.Parse(args)
	a := fl.Args()
	if len(a) == 0 {
		usage()
	}
	l, err := programnetwork.ReadPeerList(*file)
	if err != nil {
		return err
	}
	want := func(min, max int) {
		if len(a)-1 < min || (max >= 0 && len(a)-1 > max) {
			usage()
		}
	}
	switch a[0] {
	case "list":
		want(0, 0)
		os.Stdout.Write(l.Format())
		return nil
	case "shared-with":
		want(1, 1)
		for _, e := range l.SharedWith(a[1]) {
			fmt.Printf("%s %s %s\n", e.Address, e.Group, e.Scope)
		}
		return nil
	case "add":
		want(3, -1)
		class, err := programnetwork.ParseClass(a[3])
		if err != nil {
			return err
		}
		err = l.SetPeer(programnetwork.Peer{Address: a[1], Group: a[2], Class: class, Comment: strings.Join(a[4:], " ")})
		if err != nil {
			return err
		}
	case "remove":
		want(1, 1)
		if err := l.RemovePeer(a[1]); err != nil {
			return err
		}
	case "set-class", "set-group":
		want(2, 2)
		p, ok := l.Peer(a[1])
		if !ok {
			return fmt.Errorf("no peer %s", a[1])
		}
		if a[0] == "set-group" {
			p.Group = a[2]
		} else if p.Class, err = programnetwork.ParseClass(a[2]); err != nil {
			return err
		}
		if err := l.SetPeer(p); err != nil {
			return err
		}
	case "group-add":
		want(5, 5)
		scope, err := programnetwork.ParseScope(a[2])
		if err != nil {
			return err
		}
		if err := l.SetGroup(programnetwork.Group{Name: a[1], Scope: scope, Owner: a[3], Operator: a[4], SecurityGroup: a[5]}); err != nil {
			return err
		}
	case "group-remove":
		want(1, 1)
		if err := l.RemoveGroup(a[1]); err != nil {
			return err
		}
	default:
		names := []string{"list", "add", "remove", "set-class", "set-group", "group-add", "group-remove", "shared-with"}
		sort.Strings(names)
		return fmt.Errorf("unknown peers command %q, want one of %s", a[0], strings.Join(names, ", "))
	}
	return programnetwork.WritePeerList(*file, l)
}
