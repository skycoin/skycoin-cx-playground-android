// Package store reads the app bundle built into the APK. The bundle has the
// layout of programstore, the store of programs fetched over the network:
// a signed manifest list, one signed binary manifest and one linear packing
// per app, and index.txt. Every block and file is verified against its
// manifest as it is read. See docs/ProgramDistribution.md, decision D25.
package store

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// Bundle layout, relative to the bundle root, the same as programstore.
const (
	ListFile    = "list.bin"
	IndexFile   = "index.txt"
	ManifestDir = "m"
	PackingDir  = "p"
)

// BundleSeed derives the key that signs the bundle. It is public, so the
// signature only says the bundle came from cxmanifest; the APK signature
// covers the bundle's origin. The key is trusted for the bundle alone, never
// for a program from the network.
const BundleSeed = "cx playground bundle"

// BundleDate is the date in every bundle manifest, so a bundle of the same
// apps has the same bytes.
var BundleDate = time.Unix(1795392000, 0).UTC()

// BundleSigner returns the public key and secret key of BundleSeed.
func BundleSigner() (cipher.PubKey, cipher.SecKey) {
	return cipher.MustGenerateDeterministicKeyPair([]byte(BundleSeed))
}

// Source reads the files of a bundle by their path in the layout. It does
// not need to verify them.
type Source interface {
	ReadFile(name string) ([]byte, error)
}

// FSSource is a Source over a file system, such as the embedded bundle.
type FSSource struct{ FS fs.FS }

// ReadFile reads name from the file system.
func (s FSSource) ReadFile(name string) ([]byte, error) { return fs.ReadFile(s.FS, name) }

// App is a listed app: the address of its binary manifest, in hex, and the
// app manifest built from its cxapp.json.
type App struct {
	Hash     manifest.Hash
	Manifest *manifest.Manifest
}

// List verifies the bundle's manifest list and every app in it, and returns
// the apps by name. An app that fails verification is left out and its
// error returned in errs. err is set when the list itself cannot be read.
func List(src Source) (apps []App, errs []error, err error) {
	signer, _ := BundleSigner()
	listBytes, err := src.ReadFile(ListFile)
	if err != nil {
		return nil, nil, err
	}
	list, listAddr, err := decodeSigned(listBytes, programblocks.Hash{}, signer)
	if err != nil {
		return nil, nil, fmt.Errorf("manifest list: %w", err)
	}
	programs, err := packedFiles(src, listAddr, list)
	if err != nil {
		return nil, nil, fmt.Errorf("manifest list: %w", err)
	}
	for _, name := range programblocks.ProgramNames(list) {
		_, addr, err := decodeSigned(programs[name], programblocks.Hash{}, signer)
		if err != nil {
			errs = append(errs, fmt.Errorf("app %s: %w", name, err))
			continue
		}
		app := App{Hash: manifest.Hash(addr.String())}
		files, err := Load(src, app)
		if err == nil {
			app.Manifest, err = programblocks.AppManifest(files)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("app %s: %w", name, err))
			continue
		}
		apps = append(apps, app)
	}
	sort.SliceStable(apps, func(i, j int) bool { return apps[i].Manifest.Name < apps[j].Manifest.Name })
	return apps, errs, nil
}

// Load returns the files of app after checking its manifest, every block
// and every file.
func Load(src Source, app App) (map[string][]byte, error) {
	signer, _ := BundleSigner()
	addr, err := programblocks.HashFromHex(string(app.Hash))
	if err != nil {
		return nil, err
	}
	b, err := src.ReadFile(path.Join(ManifestDir, addr.String()+".bin"))
	if err != nil {
		return nil, err
	}
	m, _, err := decodeSigned(b, addr, signer)
	if err != nil {
		return nil, err
	}
	return packedFiles(src, addr, m)
}

func decodeSigned(b []byte, want programblocks.Hash, signer cipher.PubKey) (*programblocks.Manifest, programblocks.Hash, error) {
	m, addr, err := programblocks.Decode(b, want)
	if err != nil {
		return nil, addr, err
	}
	if m.SignerPublicKey != signer {
		return nil, addr, fmt.Errorf("%w: %s", programblocks.ErrUntrusted, m.SignerPublicKey.Hex())
	}
	return m, addr, nil
}

// packedFiles reads the linear packing of m, the manifest at addr, and
// returns its files once every block and file is verified.
func packedFiles(src Source, addr programblocks.Hash, m *programblocks.Manifest) (map[string][]byte, error) {
	pack, err := src.ReadFile(path.Join(PackingDir, addr.String()+".pack"))
	if err != nil {
		return nil, err
	}
	if uint64(len(pack)) != m.PackLength {
		return nil, fmt.Errorf("packing of %s has %d bytes, the manifest %d", addr, len(pack), m.PackLength)
	}
	a := programblocks.NewAssembly(m)
	if _, err := a.Accept(0, pack); err != nil {
		return nil, err
	}
	return a.Files()
}

// WriteBundle writes apps, each a map of file path to bytes with its
// cxapp.json, keyed by app name, as a bundle in dir, which it replaces. A
// signature is not repeatable, so a manifest whose address is the same as
// in previous, an earlier bundle or nil, keeps the signature it has there,
// and the bundle of the same apps has the same bytes. It returns the rows of
// index.txt.
func WriteBundle(dir string, apps map[string]map[string][]byte, previous fs.FS) ([]programstore.Row, error) {
	signer, secret := BundleSigner()
	sign := func(m *programblocks.Manifest) error {
		// The address covers the signer key but not the signature.
		m.SignerPublicKey, m.Signature = signer, cipher.Sig{}
		if previous == nil {
			return m.Sign(secret)
		}
		for _, name := range []string{path.Join(ManifestDir, m.Address().String()+".bin"), ListFile} {
			b, err := fs.ReadFile(previous, name)
			if err != nil {
				continue
			}
			if old, _, err := programblocks.Decode(b, m.Address()); err == nil && old.SignerPublicKey == signer {
				m.SignerPublicKey, m.Signature = old.SignerPublicKey, old.Signature
				return nil
			}
		}
		return m.Sign(secret)
	}
	type built struct {
		manifest *programblocks.Manifest
		encoded  []byte
		pack     []byte
	}
	var programs []built
	write := func(m *programblocks.Manifest, pack []byte) ([]byte, error) {
		if err := sign(m); err != nil {
			return nil, err
		}
		b := m.Encode()
		programs = append(programs, built{m, b, pack})
		return b, nil
	}
	encoded := map[string][]byte{}
	names := make([]string, 0, len(apps))
	for name := range apps {
		names = append(names, name)
	}
	sort.Strings(names)
	var rows []programstore.Row
	for i, name := range names {
		m, pack, err := programblocks.Build(apps[name], programblocks.DefaultBlockSize, BundleDate)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if encoded[name], err = write(m, pack); err != nil {
			return nil, err
		}
		rows = append(rows, programstore.Row{Number: i + 1, Address: m.Address().String(), State: programstore.StateCached, Length: int64(m.PackLength), Name: name})
	}
	list, listPack, err := programblocks.BuildManifestList(encoded, BundleDate)
	if err != nil {
		return nil, err
	}
	listEncoded, err := write(list, listPack)
	if err != nil {
		return nil, err
	}

	if err := os.RemoveAll(dir); err != nil {
		return nil, err
	}
	for _, d := range []string{ManifestDir, PackingDir} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return nil, err
		}
	}
	for _, p := range programs {
		addr := p.manifest.Address().String()
		if err := os.WriteFile(filepath.Join(dir, ManifestDir, addr+".bin"), p.encoded, 0o644); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, PackingDir, addr+".pack"), p.pack, 0o644); err != nil {
			return nil, err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ListFile), listEncoded, 0o644); err != nil {
		return nil, err
	}
	var index strings.Builder
	for _, r := range rows {
		index.WriteString(r.Line() + "\n")
	}
	return rows, os.WriteFile(filepath.Join(dir, IndexFile), []byte(index.String()), 0o644)
}

// WriteDataFiles writes the files of an app that are not CX sources under
// dir, the sandbox root of its program, and reports whether it wrote any.
func WriteDataFiles(dir string, files map[string][]byte) (bool, error) {
	wrote := false
	for name, b := range files {
		if strings.HasSuffix(name, ".cx") {
			continue
		}
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return wrote, err
		}
		if err := os.WriteFile(p, b, 0o600); err != nil {
			return wrote, err
		}
		wrote = true
	}
	return wrote, nil
}
