package main

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programcheck"
)

// fetchKind marks an app that fetches its program over UDP instead of
// holding it. Its args are the server address, the trusted signer public
// keys in hex, comma separated, and the program name, then optional
// settings packet=BYTES, forms=full|short and tries=N, see
// docs/ProgramDistribution.md. Any valid signer is accepted, and the user
// confirms a program from a key that is not trusted before it runs (D27).
const fetchKind = "fetch"

// fetchClient returns a client for conn with the settings of args.
func fetchClient(conn net.Conn, args []string) (*programblocks.Client, error) {
	c := programblocks.NewClient(conn)
	for _, a := range args {
		key, value, _ := strings.Cut(a, "=")
		if key == "forms" {
			forms, err := programblocks.ParseForms(value)
			if err != nil {
				return nil, err
			}
			c.Forms = forms
			continue
		}
		n, err := strconv.Atoi(value)
		switch {
		case err != nil:
			return nil, fmt.Errorf("fetch setting %q: want key=number", a)
		case key == "packet":
			if err := programblocks.CheckPacketSize(n); err != nil {
				return nil, err
			}
			c.PacketSize = n
		case key == "tries" && n > 0:
			c.MaxTries = n
		default:
			return nil, fmt.Errorf("unknown fetch setting %q, want packet=BYTES, forms=full|short or tries=N", a)
		}
	}
	return c, nil
}

// fetchProgram fetches and verifies the program a fetch app names and
// returns its app manifest and files, reporting each step to out. confirm
// asks the user about a signer that is not trusted.
func fetchProgram(m *manifest.Manifest, out io.Writer, confirm func(cipher.PubKey) bool) (*manifest.Manifest, map[string][]byte, error) {
	if len(m.Args) < 3 {
		return nil, nil, fmt.Errorf("a fetch app takes the server, the signer key and the program name as args")
	}
	server, name := m.Args[0], m.Args[2]
	trusted, err := parseKeys(m.Args[1])
	if err != nil {
		return nil, nil, err
	}
	conn, err := net.Dial("udp", server)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()
	start := time.Now()
	c, err := fetchClient(conn, m.Args[3:])
	if err != nil {
		return nil, nil, err
	}
	if c.PacketSize != programblocks.DefaultPacketSize || c.Forms != programblocks.FormsFull {
		forms := "full"
		if c.ShortForms() {
			forms = "short"
		}
		fmt.Fprintf(out, "packet size %d bytes, %s forms\n", c.PacketSize, forms)
	}
	p, err := c.FetchProgram(name)
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(out, "fetched %s from %s in %d ms\n", name, server, time.Since(start).Milliseconds())
	fmt.Fprintf(out, "manifest %s\n", p.Address)
	if err := reportSigner(out, p.Manifest.SignerPublicKey, trusted, confirm); err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(out, "%d files, %d blocks, %d bytes, every block verified\n", len(p.Manifest.Files), len(p.Manifest.Blocks), p.Manifest.PackLength)
	fmt.Fprintf(out, "packets %d requests, %d resent, largest reply %d bytes\n", c.Stats.RequestsSent, c.Stats.Retransmissions, c.Stats.LargestReply)
	app, err := programblocks.AppManifest(p.Files)
	if err != nil {
		return nil, nil, err
	}
	return app, p.Files, nil
}

// checkedProgram checks a program that came over the network against
// programcheck.PolicyFetched, writes the report to out and refuses to run
// the program when the report has a violation.
func checkedProgram(m *manifest.Manifest, files map[string][]byte, out io.Writer) (*preparedProgram, error) {
	policy := programcheck.PolicyFetched()
	r, err := programcheck.CheckApp(m, files, policy)
	if err != nil {
		return nil, fmt.Errorf("check failed: %v", err)
	}
	fmt.Fprintf(out, "check of %s before it runs:\n%s", m.Name, r.Text())
	if !r.Allowed() {
		return nil, fmt.Errorf("refused to run %s: %d sandbox violations", m.Name, len(r.Violations))
	}
	return &preparedProgram{app: m, files: files, libraries: r.LibrariesToEnable(policy)}, nil
}

// parseKeys parses comma separated public keys in hex.
func parseKeys(s string) ([]cipher.PubKey, error) {
	var keys []cipher.PubKey
	for _, h := range strings.Split(s, ",") {
		k, err := cipher.PubKeyFromHex(strings.TrimSpace(h))
		if err != nil {
			return nil, fmt.Errorf("signer %q: %v", h, err)
		}
		keys = append(keys, k)
	}
	return keys, nil
}
