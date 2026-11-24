package main

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programcheck"
)

// fetchKind marks an app that fetches its program over UDP instead of
// holding it. Its args are the server address, the trusted signer public
// key in hex and the program name, see docs/ProgramDistribution.md.
const fetchKind = "fetch"

// fetchProgram fetches and verifies the program a fetch app names and
// returns its app manifest and files, reporting each step to out.
func fetchProgram(m *manifest.Manifest, out io.Writer) (*manifest.Manifest, map[string][]byte, error) {
	if len(m.Args) != 3 {
		return nil, nil, fmt.Errorf("a fetch app takes the server, the signer key and the program name as args")
	}
	server, signerHex, name := m.Args[0], m.Args[1], m.Args[2]
	signer, err := cipher.PubKeyFromHex(signerHex)
	if err != nil {
		return nil, nil, err
	}
	conn, err := net.Dial("udp", server)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()
	start := time.Now()
	c := programblocks.NewClient(conn, signer)
	p, err := c.FetchProgram(name)
	if err != nil {
		return nil, nil, err
	}
	fmt.Fprintf(out, "fetched %s from %s in %d ms\n", name, server, time.Since(start).Milliseconds())
	fmt.Fprintf(out, "manifest %s\n", p.Address)
	fmt.Fprintf(out, "signer %s verified\n", p.Manifest.SignerPublicKey.Hex())
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
