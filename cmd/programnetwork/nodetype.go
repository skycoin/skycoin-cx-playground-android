package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

// nodeSettings reads the config file, if any, and lets the flags set on
// the command line override it.
func nodeSettings(fl *flag.FlagSet, configFile, typeName string, budget int64, pinsFile string) (programnetwork.Settings, error) {
	values := map[string]string{}
	if configFile != "" {
		lines, err := programstore.ReadLines(configFile)
		if err != nil {
			return programnetwork.Settings{}, err
		}
		if len(lines) == 0 {
			if _, err := os.Stat(configFile); err != nil {
				return programnetwork.Settings{}, err
			}
		}
		for i, l := range lines {
			key, value, _ := strings.Cut(l, " ")
			value = strings.TrimSpace(value)
			switch key {
			case "type", "budget", "pins":
				values[key] = value
			default:
				return programnetwork.Settings{}, fmt.Errorf("%s entry %d: unknown setting %q, want type, budget or pins", configFile, i+1, key)
			}
		}
	}
	fl.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "type", "budget", "pins":
			values[f.Name] = f.Value.String()
		}
	})
	if _, ok := values["type"]; !ok {
		values["type"] = typeName
	}
	if _, ok := values["budget"]; !ok {
		values["budget"] = strconv.FormatInt(budget, 10)
	}
	if _, ok := values["pins"]; !ok {
		values["pins"] = pinsFile
	}
	t, err := programnetwork.ParseNodeType(values["type"])
	if err != nil {
		return programnetwork.Settings{}, err
	}
	b, err := strconv.ParseInt(values["budget"], 10, 64)
	if err != nil || b < 0 {
		return programnetwork.Settings{}, fmt.Errorf("budget %q: want bytes, 0 or more", values["budget"])
	}
	return programnetwork.Settings{Type: t, Budget: b, PinsFile: values["pins"]}, nil
}

func pins(args []string) error {
	fl := flag.NewFlagSet("pins", flag.ExitOnError)
	file := fl.String("file", "pins.txt", "pin list file")
	fl.Parse(args)
	a := fl.Args()
	if len(a) == 0 {
		usage()
	}
	list, err := programnetwork.ReadPins(*file)
	if err != nil {
		return err
	}
	var given []string
	for _, s := range a[1:] {
		p, err := programnetwork.ParsePin(s)
		if err != nil {
			return err
		}
		given = append(given, p)
	}
	switch a[0] {
	case "list":
		for _, p := range list {
			fmt.Println(p)
		}
		return nil
	case "add":
		if len(given) == 0 {
			usage()
		}
		list = append(list, given...)
	case "remove":
		if len(given) == 0 {
			usage()
		}
		for _, p := range given {
			if !slices.Contains(list, p) {
				return fmt.Errorf("no pin %s in %s", p, *file)
			}
		}
		list = slices.DeleteFunc(list, func(p string) bool { return slices.Contains(given, p) })
	default:
		return fmt.Errorf("unknown pins command %q, want add, list or remove", a[0])
	}
	return programnetwork.WritePins(*file, list)
}

func storeCommand(args []string) error {
	fl := flag.NewFlagSet("store", flag.ExitOnError)
	dir := fl.String("store", "programnetwork-store", "store directory")
	fl.Parse(args)
	a := fl.Args()
	if len(a) == 0 {
		usage()
	}
	st, err := programstore.Open(*dir)
	if err != nil {
		return err
	}
	switch a[0] {
	case "list":
		rows, err := st.ReadIndex()
		if err != nil {
			return err
		}
		for _, r := range rows {
			fmt.Println(r.Line())
		}
		return nil
	case "remove":
		if len(a) != 2 {
			usage()
		}
		prefix, err := programnetwork.ParsePin(a[1])
		if err != nil {
			return err
		}
		row, err := st.FindRow(prefix)
		if err != nil {
			return err
		}
		addr, err := programblocks.HashFromHex(row.Address)
		if err != nil {
			return err
		}
		if err := st.Remove(addr); err != nil {
			return err
		}
		fmt.Printf("removed %s %s; a running node serves it until it restarts\n", row.Address, row.Name)
		return nil
	}
	return fmt.Errorf("unknown store command %q, want list or remove", a[0])
}
