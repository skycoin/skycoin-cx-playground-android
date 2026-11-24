package programstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
)

// TrustedSigners reads trusted.txt: one public key in hex per line, then
// an optional comment after a space. Empty lines and lines starting with #
// are skipped. Only manifests signed by these keys are accepted, decision
// D27 of docs/ProgramDistribution.md.
func (s *Store) TrustedSigners() ([]cipher.PubKey, error) {
	b, err := os.ReadFile(s.TrustedPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []cipher.PubKey
	for n, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		hex, _, _ := strings.Cut(line, " ")
		k, err := cipher.PubKeyFromHex(hex)
		if err != nil {
			return nil, fmt.Errorf("trusted.txt line %d: %v", n+1, err)
		}
		keys = append(keys, k)
	}
	return keys, nil
}

// TrustSigner adds key to trusted.txt with a comment, unless it is there.
func (s *Store) TrustSigner(key cipher.PubKey, comment string) error {
	keys, err := s.TrustedSigners()
	if err != nil {
		return err
	}
	for _, k := range keys {
		if k == key {
			return nil
		}
	}
	b, err := os.ReadFile(s.TrustedPath())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		b = append(b, '\n')
	}
	line := key.Hex()
	if comment != "" {
		line += " " + comment
	}
	return writeFileSynced(s.TrustedPath(), append(b, line+"\n"...))
}

// Sync fetches the manifest list of the server c talks to and every program
// manifest in it, verifies them against the trusted signers and keeps them,
// and returns the new index rows.
func (s *Store) Sync(c *programblocks.Client) ([]Row, error) {
	trusted, err := s.TrustedSigners()
	if err != nil {
		return nil, err
	}
	if len(trusted) == 0 {
		return nil, fmt.Errorf("no trusted signer in %s", s.TrustedPath())
	}
	c.TrustedSigners = trusted
	listAddr, _, err := c.ListManifests()
	if err != nil {
		return nil, err
	}
	list, encoded, err := c.FetchManifest(listAddr)
	if err != nil {
		return nil, fmt.Errorf("manifest list: %w", err)
	}
	programs, err := c.FetchFiles(listAddr, list)
	if err != nil {
		return nil, fmt.Errorf("manifest list: %w", err)
	}
	return s.SaveList(encoded, programs, trusted)
}
