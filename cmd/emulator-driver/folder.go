package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// settingsFile is the optional per test settings file of an emulator test.
//
//	app <manifest name>        run a bundled app from apps/ instead of app/
//	ready <text>               wait for a test mode line containing text
//	                           instead of the program's first frame
//	setprop <name> <value>     set a system property before the start, and
//	                           clear it after the test
//	before <shell command>     run on the device before the start
//	after <shell command>      run on the device after the test
//	echo-server, program-server, program-packet, program-nodes, program-node-type, netem, capture see net.go
//	input untimed              send input.txt events without timing them, for
//	                           a screen of the app that is not a program canvas
//	timeout <seconds>          for wait-for steps and the expected output,
//	                           10 by default
const settingsFile = "emulator.txt"

// testFolder is one emulator test under tests/: app/ holds the CX app
// (bundled into the APK by make bundle), input.txt the events, expect.txt
// the expected output, expected/ the reference screenshots and emulator.txt
// optional settings.
type testFolder struct {
	dir     string
	name    string // folder name
	appName string // manifest name, used to autostart the app
	ready   string // trace text that marks the app ready, "" for a frame
	// untimedInput sends events without waiting for the program to see them.
	untimedInput bool
	props        [][2]string
	before       []string
	after        []string
	timeout      time.Duration
	net          netSettings
	steps        []inputStep
	expect       []expectation
}

// errNotEmulatorTest marks a folder under tests/ that holds go tests only.
var errNotEmulatorTest = errors.New("not an emulator test")

// emulatorTest reports whether dir is an emulator test: it has an app/ or
// an emulator.txt. Other folders under tests/ hold go tests.
func emulatorTest(dir string) bool {
	for _, name := range []string{"app", settingsFile} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func loadTestFolder(dir string) (testFolder, error) {
	t := testFolder{dir: dir, name: filepath.Base(dir), timeout: 10 * time.Second}
	if !emulatorTest(dir) {
		return t, errNotEmulatorTest
	}
	if b, err := os.ReadFile(filepath.Join(dir, "app", "cxapp.json")); err == nil {
		var app struct{ Name string }
		if err := json.Unmarshal(b, &app); err != nil {
			return t, fmt.Errorf("%s: %w", dir, err)
		}
		t.appName = app.Name
	} else if !errors.Is(err, fs.ErrNotExist) {
		return t, err
	}
	parsers := map[string]func(io.Reader) error{
		"input.txt":  func(r io.Reader) (err error) { t.steps, err = parseSteps(r); return },
		"expect.txt": func(r io.Reader) (err error) { t.expect, err = parseExpectations(r); return },
		settingsFile: t.parseSettings,
	}
	for _, file := range []string{settingsFile, "input.txt", "expect.txt"} {
		f, err := os.Open(filepath.Join(dir, file))
		if errors.Is(err, fs.ErrNotExist) && file == settingsFile {
			continue
		}
		if err != nil {
			return t, err
		}
		err = parsers[file](f)
		f.Close()
		if err != nil {
			return t, fmt.Errorf("%s/%s: %w", dir, file, err)
		}
	}
	if t.appName == "" {
		return t, fmt.Errorf("%s: no app/ and no app line in %s", dir, settingsFile)
	}
	return t, nil
}

func (t *testFolder) parseSettings(r io.Reader) error {
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		verb, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return fmt.Errorf("line %d: %s needs a value", n, verb)
		}
		switch verb {
		case "app":
			t.appName = rest
		case "ready":
			t.ready = rest
		case "setprop":
			name, value, _ := strings.Cut(rest, " ")
			if value = strings.TrimSpace(value); value == "" {
				return fmt.Errorf("line %d: setprop takes a name and a value", n)
			}
			t.props = append(t.props, [2]string{name, value})
		case "before":
			t.before = append(t.before, rest)
		case "after":
			t.after = append(t.after, rest)
		case "echo-server":
			t.net.echoPort = rest
		case "program-server":
			f := strings.Fields(rest)
			if len(f) < 3 {
				return fmt.Errorf("line %d: program-server takes a port, a directory and a seed", n)
			}
			t.net.programPort, t.net.programDir = f[0], filepath.Join(t.dir, f[1])
			t.net.programSeed = strings.Join(f[2:], " ")
		case "program-packet":
			if s, err := strconv.Atoi(rest); err != nil || s < 16 || s > 65507 {
				return fmt.Errorf("line %d: program-packet takes a packet size of 16 to 65507 bytes", n)
			}
			t.net.programPacket = rest
		case "program-nodes":
			f := strings.Fields(rest)
			if len(f) < 4 {
				return fmt.Errorf("line %d: program-nodes takes a first port, a count, a directory and a seed", n)
			}
			port, err := strconv.Atoi(f[0])
			count, cerr := strconv.Atoi(f[1])
			if err != nil || cerr != nil || port <= 0 || count < 1 || port+count > 65536 {
				return fmt.Errorf("line %d: program-nodes takes a port and a node count", n)
			}
			t.net.nodesPort, t.net.nodesCount, t.net.nodesDir = port, count, filepath.Join(t.dir, f[2])
			t.net.nodesSeed = strings.Join(f[3:], " ")
		case "program-node-type":
			if rest != "medium" && rest != "super" {
				return fmt.Errorf("line %d: program-node-type takes medium or super", n)
			}
			t.net.nodesType = rest
		case "input":
			if rest != "untimed" {
				return fmt.Errorf("line %d: input takes untimed", n)
			}
			t.untimedInput = true
		case "netem":
			t.net.netem = rest
		case "capture":
			t.net.capture = rest
		case "timeout":
			s, err := strconv.Atoi(rest)
			if err != nil || s <= 0 {
				return fmt.Errorf("line %d: timeout takes seconds", n)
			}
			t.timeout = time.Duration(s) * time.Second
		default:
			return fmt.Errorf("line %d: unknown setting %q", n, verb)
		}
	}
	return sc.Err()
}
