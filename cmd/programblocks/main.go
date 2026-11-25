// Command programblocks serves a directory of CX programs over UDP as signed
// manifests and blocks, and fetches and verifies one, as described in
// docs/ProgramDistribution.md.
//
//	programblocks serve [-addr :7002] [-packet 1200] (-seed TEXT | -secret HEX) DIR
//	programblocks fetch [-addr HOST:7002] [-packet 1200] [-forms full] [-signer PUBKEY,...] [-yes] [-out DIR] [-text] PROGRAM
//	programblocks list  [-addr HOST:7002] [-packet 1200] [-forms full]
//	programblocks signer (-seed TEXT | -secret HEX)
//
// Every subdirectory of DIR is a program named after it, with its
// cxapp.txt. A seed derives the signing key with the Skycoin cipher
// package; a public seed is for tests only.
//
// fetch accepts a program with a valid signature from any key and prints
// the key (D27). A key in -signer is trusted, and a program from any other
// key is kept only with -yes, the host form of the app's question.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "fetch", "list":
		err = fetch(os.Args[1], os.Args[2:])
	case "signer":
		err = signer(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "programblocks:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: programblocks serve|fetch|list|signer, see go doc ./cmd/programblocks")
	os.Exit(2)
}

func secretFlags(fl *flag.FlagSet) func() (cipher.SecKey, error) {
	seed := fl.String("seed", "", "derive the signing key from this seed (tests only when public)")
	secret := fl.String("secret", "", "signing secret key in hex")
	return func() (cipher.SecKey, error) {
		switch {
		case *secret != "":
			return cipher.SecKeyFromHex(*secret)
		case *seed != "":
			_, sk, err := cipher.GenerateDeterministicKeyPair([]byte(*seed))
			return sk, err
		}
		return cipher.SecKey{}, fmt.Errorf("give -seed or -secret")
	}
}

func signer(args []string) error {
	fl := flag.NewFlagSet("signer", flag.ExitOnError)
	secret := secretFlags(fl)
	fl.Parse(args)
	sk, err := secret()
	if err != nil {
		return err
	}
	pk, err := cipher.PubKeyFromSecKey(sk)
	if err != nil {
		return err
	}
	fmt.Println(pk.Hex())
	return nil
}

func serve(args []string) error {
	fl := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fl.String("addr", ":7002", "UDP address to listen on")
	packet := fl.Int("packet", programblocks.DefaultPacketSize, "largest packet in bytes, 1200 by default, 16 to 65507")
	secret := secretFlags(fl)
	fl.Parse(args)
	if fl.NArg() != 1 {
		usage()
	}
	sk, err := secret()
	if err != nil {
		return err
	}
	catalog, err := programblocks.CatalogFromDirectory(fl.Arg(0), sk, time.Now())
	if err != nil {
		return err
	}
	conn, err := net.ListenPacket("udp", *addr)
	if err != nil {
		return err
	}
	pk, _ := cipher.PubKeyFromSecKey(sk)
	fmt.Printf("serving %d manifests on %s, packet %d bytes, signer %s, manifest list %s\n",
		len(catalog.Addresses()), conn.LocalAddr(), *packet, pk.Hex(), catalog.ListAddress)
	return catalog.Serve(conn, *packet)
}

func fetch(cmd string, args []string) error {
	fl := flag.NewFlagSet(cmd, flag.ExitOnError)
	addr := fl.String("addr", "127.0.0.1:7002", "server UDP address")
	packet := fl.Int("packet", programblocks.DefaultPacketSize, "largest packet in bytes, 1200 by default, 16 to 65507")
	formsName := fl.String("forms", "full", "request forms, full or short; a reply that cannot fit fails with error 6")
	signers := fl.String("signer", "", "comma separated public keys, hex, of trusted signers")
	yes := fl.Bool("yes", false, "keep a program whose signer is not in -signer")
	out := fl.String("out", "", "directory to write the program's files to")
	text := fl.Bool("text", false, "print the program manifest in text form")
	fl.Parse(args)
	if (cmd == "fetch") != (fl.NArg() == 1) {
		usage()
	}
	trusted, err := parseKeys(*signers)
	if err != nil {
		return err
	}
	conn, err := net.Dial("udp", *addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	c := programblocks.NewClient(conn)
	c.PacketSize = *packet
	if c.Forms, err = programblocks.ParseForms(*formsName); err != nil {
		return err
	}
	start := time.Now()
	if cmd == "list" {
		list, _, _, err := c.FetchManifestList()
		if err != nil {
			return err
		}
		for _, name := range programblocks.ProgramNames(list) {
			fmt.Println(name)
		}
		return nil
	}
	p, err := c.FetchProgram(fl.Arg(0))
	if err != nil {
		return err
	}
	fmt.Printf("fetched %s: manifest %s, %d files, %d blocks, %d bytes, verified in %d ms\n",
		fl.Arg(0), p.Address, len(p.Manifest.Files), len(p.Manifest.Blocks), p.Manifest.PackLength, time.Since(start).Milliseconds())
	fmt.Printf("packets: %d requests, %d resent, %d replies, largest %d bytes\n",
		c.Stats.RequestsSent, c.Stats.Retransmissions, c.Stats.RepliesAccepted, c.Stats.LargestReply)
	if err := confirmSigner(p.Manifest.SignerPublicKey, trusted, *yes); err != nil {
		return err
	}
	if *text {
		os.Stdout.Write(p.Manifest.EncodeText())
	}
	if *out != "" {
		for path, b := range p.Files {
			dst := filepath.Join(*out, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dst, b, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// parseKeys parses comma separated public keys in hex, none for "".
func parseKeys(s string) ([]cipher.PubKey, error) {
	var keys []cipher.PubKey
	for _, h := range strings.Split(s, ",") {
		if h = strings.TrimSpace(h); h == "" {
			continue
		}
		k, err := cipher.PubKeyFromHex(h)
		if err != nil {
			return nil, fmt.Errorf("-signer %s: %v", h, err)
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// confirmSigner prints the signer of a verified program and refuses it
// when it is not trusted and yes is not set (D27).
func confirmSigner(signer cipher.PubKey, trusted []cipher.PubKey, yes bool) error {
	if programblocks.KeyListed(trusted, signer) {
		fmt.Printf("signer %s verified, trusted\n", signer.Hex())
		return nil
	}
	fmt.Printf("signer %s verified, not in -signer\n", signer.Hex())
	if err := programblocks.ConfirmSigner(signer, trusted, func(cipher.PubKey) bool { return yes }); err != nil {
		return fmt.Errorf("%w, pass -yes to keep it", err)
	}
	return nil
}
