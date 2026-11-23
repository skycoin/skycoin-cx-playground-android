// Command cxmanifest builds and checks CX app manifests and bundles.
//
//	cxmanifest build [-chunk N] DIR       print the manifest of DIR, app hash on stderr
//	cxmanifest hash DIR                   print the app hash of DIR
//	cxmanifest bundle -o OUT DIR...       write a bundle (index, manifests, chunks)
//	cxmanifest verify BUNDLE              verify every app in a bundle
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/skycoin/skycoin-cx-playground-android/manifest"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "build", "hash":
		err = build(os.Args[1], os.Args[2:])
	case "bundle":
		err = bundle(os.Args[2:])
	case "verify":
		err = verify(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "cxmanifest:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: cxmanifest build|hash [-chunk N] DIR | bundle -o OUT DIR... | verify BUNDLE")
	os.Exit(2)
}

func build(cmd string, args []string) error {
	fl := flag.NewFlagSet(cmd, flag.ExitOnError)
	chunk := fl.Int("chunk", manifest.DefaultChunkSize, "chunk size in bytes")
	fl.Parse(args)
	if fl.NArg() != 1 {
		usage()
	}
	m, _, err := manifest.BuildDir(fl.Arg(0), *chunk)
	if err != nil {
		return err
	}
	b, err := m.Encode()
	if err != nil {
		return err
	}
	h := manifest.Sum(b)
	if cmd == "hash" {
		fmt.Println(h)
		return nil
	}
	os.Stdout.Write(b)
	fmt.Fprintln(os.Stderr, h)
	return nil
}

func bundle(args []string) error {
	fl := flag.NewFlagSet("bundle", flag.ExitOnError)
	out := fl.String("o", "", "output directory, replaced")
	chunk := fl.Int("chunk", manifest.DefaultChunkSize, "chunk size in bytes")
	fl.Parse(args)
	if *out == "" || fl.NArg() == 0 {
		usage()
	}
	var apps []store.Built
	for _, dir := range fl.Args() {
		m, chunks, err := manifest.BuildDir(dir, *chunk)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		apps = append(apps, store.Built{Manifest: m, Chunks: chunks})
	}
	if err := os.RemoveAll(*out); err != nil {
		return err
	}
	hashes, err := store.WriteBundle(*out, apps)
	if err != nil {
		return err
	}
	for i, h := range hashes {
		fmt.Printf("%s  %s\n", h, apps[i].Manifest.Name)
	}
	return nil
}

func verify(args []string) error {
	if len(args) != 1 {
		usage()
	}
	src := store.FSSource{FS: os.DirFS(args[0])}
	apps, errs, err := store.List(src)
	if err != nil {
		return err
	}
	for _, a := range apps {
		if _, err := store.Load(src, a); err != nil {
			errs = append(errs, fmt.Errorf("app %s: %w", a.Hash, err))
			continue
		}
		fmt.Printf("ok  %s  %s\n", a.Hash, a.Manifest.Name)
	}
	for _, e := range errs {
		fmt.Println("BAD", e)
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d apps failed", len(errs))
	}
	return nil
}
