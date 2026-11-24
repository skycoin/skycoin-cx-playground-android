// Command programnetwork runs a node of the DHT prototype of #18, gets one
// program as a thin node, and manages its peer list file, as described in
// "DHT prototype (#18)" in docs/ProgramDistribution.md.
//
//	programnetwork node [-addr :7010] [-store DIR] [-peers FILE] [-packet 1200] [-forms full]
//	                    [-signer PUBKEY,...] [-subscribe HOST:PORT,...]
//	                    [-publish DIR (-seed TEXT | -secret HEX) [-date RFC3339]]
//	                    [-config FILE] [-type medium|super] [-budget BYTES]
//	                    [-pins FILE] [-relay] [-fetch HASH,...]
//	programnetwork get -signer PUBKEY [-packet 1200] [-forms full] [-out DIR] ADDRESS NODE
//	programnetwork getpeers HOST:PORT
//	programnetwork pins [-file pins.txt] list
//	programnetwork pins [-file pins.txt] add HASH-OR-PREFIX...
//	programnetwork pins [-file pins.txt] remove HASH-OR-PREFIX...
//	programnetwork store [-store DIR] list
//	programnetwork store [-store DIR] remove HASH-PREFIX
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
// a program and announced to the subscribers. Classes are private,
// own-infrastructure, lan-group, user-shareable and public; scopes are
// lan, internal and global.
//
// No have starts a download (#42). A node puts every have it receives in
// its have queue, prints each one as it reads it, and its type's policy
// requests what it keeps. The node type, "Node types (#34, #44)" in
// docs/ProgramDistribution.md, is super by default. A thin node is the get
// command: it connects to NODE, downloads ADDRESS (a whole address, or a
// prefix with -forms short), verifies it and exits, with no subscription
// and no state. A medium node keeps a subscription to its -subscribe
// nodes, requests the programs its -pins file names and others within
// -budget, and relays to its own subscribers only with -relay. A super node
// requests everything up to -budget, 0 for no limit, never evicts, and
// relays: `store remove` deletes a program, and a running node keeps
// serving it until it restarts. -config names a file of "type medium",
// "budget 1000000", "pins pins.txt" and "relay on" lines, which the flags
// override.
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
	case "get":
		err = get(os.Args[2:])
	case "getpeers":
		err = getPeers(os.Args[2:])
	case "peers":
		err = peers(os.Args[2:])
	case "pins":
		err = pins(os.Args[2:])
	case "store":
		err = storeCommand(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "programnetwork:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: programnetwork node|get|getpeers|peers|pins|store, see go doc ./cmd/programnetwork")
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
	formsName := fl.String("forms", "full", "request forms of downloads and get peers, full or short; a reply that cannot fit fails with error 6")
	signers := fl.String("signer", "", "comma separated public keys, hex, whose programs the node downloads")
	subscribe := fl.String("subscribe", "", "comma separated nodes to subscribe to")
	publish := fl.String("publish", "", "directory of program directories to publish")
	seed := fl.String("seed", "", "derive the publishing key from this seed (tests only when public)")
	secretHex := fl.String("secret", "", "publishing secret key in hex")
	configFile := fl.String("config", "", "file of type, budget, pins and relay lines, overridden by the flags")
	typeName := fl.String("type", string(programnetwork.TypeSuper), "node type: medium or super; a thin node is the get command")
	budget := fl.Int64("budget", 0, "bytes a medium node keeps besides its pins, or a super node's capacity, 0 for none")
	pinsFile := fl.String("pins", "", "a medium node's pin list file, one hash or prefix per line")
	fetch := fl.String("fetch", "", "comma separated manifest addresses to request on demand, from the -subscribe nodes")
	relay := fl.Bool("relay", false, "a medium node accepts subscribers and relays the haves of what it holds; a super node always relays")
	date := fl.String("date", "", "publish date in RFC 3339, now when empty")
	fl.Parse(args)
	settings, err := nodeSettings(fl, *configFile, *typeName, *budget, *pinsFile, *relay)
	if err != nil {
		return err
	}
	if settings.Type == programnetwork.TypeThin {
		return fmt.Errorf("a thin node connects, downloads and is done: use programnetwork get ADDRESS NODE")
	}
	publishDate := time.Now()
	if *date != "" {
		if publishDate, err = time.Parse(time.RFC3339, *date); err != nil {
			return fmt.Errorf("-date: %v", err)
		}
	}
	var trusted []cipher.PubKey
	for _, s := range splitList(*signers) {
		k, err := cipher.PubKeyFromHex(s)
		if err != nil {
			return fmt.Errorf("-signer %s: %v", s, err)
		}
		trusted = append(trusted, k)
	}
	var secret cipher.SecKey
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
	n, err := programnetwork.NewTypedNode(conn, st, trusted, settings)
	if err != nil {
		return err
	}
	n.PacketSize = *packet
	if n.Forms, err = programblocks.ParseForms(*formsName); err != nil {
		return err
	}
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
	fmt.Printf("node listening on %s, packet %d bytes, forms %s, %d trusted signers, type %s, budget %d bytes, pins %q, relay %t, %d bytes held\n",
		conn.LocalAddr(), *packet, n.Forms, len(trusted), settings.Type, settings.Budget, settings.PinsFile, settings.Relay, n.Stats().StoredBytes)
	done := make(chan error, 1)
	go func() { done <- n.Run() }()
	go n.RunUpdates(func(q programnetwork.QueuedHave) {
		n.Logf("have %s from %s, %d manifest bytes, %d packing bytes", q.Address, q.From, q.ManifestLength, q.PackLength)
	})
	for _, s := range splitList(*subscribe) {
		if err := n.Subscribe(s); err != nil {
			return fmt.Errorf("subscribe to %s: %v", s, err)
		}
	}
	for _, f := range splitList(*fetch) {
		addr, err := programblocks.HashFromHex(f)
		if err != nil {
			return fmt.Errorf("-fetch %s: %v", f, err)
		}
		p, err := n.Fetch(addr, splitList(*subscribe)...)
		if err != nil {
			return err
		}
		fmt.Printf("fetched %s %s, %d files, %d bytes\n", p.Name, p.Address, len(p.Files), p.Manifest.PackLength)
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
			if _, err := n.Publish(files, secret, publishDate); err != nil {
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
	n, err := programnetwork.NewTypedNode(conn, st, nil, programnetwork.Settings{Type: programnetwork.TypeMedium})
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

// get is the thin node of #44: connect, download, verify, done.
func get(args []string) error {
	fl := flag.NewFlagSet("get", flag.ExitOnError)
	packet := fl.Int("packet", programblocks.DefaultPacketSize, "largest packet in bytes, 1200 by default, 16 to 65507")
	formsName := fl.String("forms", "full", "request forms, full or short; an address prefix needs short")
	signers := fl.String("signer", "", "comma separated public keys, hex, whose programs are accepted")
	out := fl.String("out", "", "directory to write the program's files to")
	fl.Parse(args)
	if fl.NArg() != 2 || *signers == "" {
		return fmt.Errorf("usage: programnetwork get -signer PUBKEY [-packet 1200] [-forms full] [-out DIR] ADDRESS NODE")
	}
	o := programnetwork.GetOptions{PacketSize: *packet}
	for _, s := range splitList(*signers) {
		k, err := cipher.PubKeyFromHex(s)
		if err != nil {
			return fmt.Errorf("-signer %s: %v", s, err)
		}
		o.Trusted = append(o.Trusted, k)
	}
	var err error
	if o.Forms, err = programblocks.ParseForms(*formsName); err != nil {
		return err
	}
	p, stats, err := programnetwork.Get(fl.Arg(1), fl.Arg(0), o)
	if err != nil {
		return err
	}
	fmt.Printf("got %s %s from %s in %d ms, %d files, %d bytes, verified, %d requests\n",
		p.Name, p.Address, p.From, p.Elapsed.Milliseconds(), len(p.Files), p.Manifest.PackLength, stats.RequestsSent)
	if *out == "" {
		return nil
	}
	for path, b := range p.Files {
		dst := filepath.Join(*out, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
	}
	return nil
}
