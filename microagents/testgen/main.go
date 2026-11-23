// Command testgen writes the generated test folders of a micro agent from
// its table, tests.table: one folder per tool, one per ordered pair of
// tools, and one per chain. Each folder gets inbound.txt, the spiffs/
// fixture and a run.sh that calls testlib/tool-test.sh. A folder whose
// inputs the table changes is written again without its recording, a
// folder the table no longer has is removed, and the folders that need
// recording are printed for testlib/record-generated.sh.
//
//	testgen -agent femtoclaw
//	testgen -agent femtoclaw -check
//
// The table has one tab separated line per entry, with the \n, \t and \\
// escapes of inbound.txt in the last field:
//
//	inbound	telegram	3003            fields put before each message
//	file	notes.md	text            a fixture file below spiffs/
//	pair	read_file	{"path":...}    the call a pair test makes of a tool
//	case	read_file	message         a message of the test of a tool
//	chain	copy-file	message         a message of a chain test
//
// "{uptime}" in an inbound field becomes 1000 times the message number.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type table struct {
	inbound []string
	files   map[string]string
	pairs   []pairCall
	cases   map[string][]string
	chains  map[string][]string
	tools   []string
	order   []string
}

type pairCall struct {
	tool string
	args string
}

func main() {
	agent := flag.String("agent", "", "agent folder, for example femtoclaw")
	dir := flag.String("dir", ".", "the microagents folder")
	check := flag.Bool("check", false, "only check that the folders match the table")
	flag.Parse()
	var err error
	if *check {
		err = checkFolders(*dir, *agent)
	} else {
		err = update(*dir, *agent)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "testgen:", err)
		os.Exit(1)
	}
}

// generate writes the folders of the table of agent into the empty folder
// testsDir and returns them.
func generate(dir, agent, testsDir string) ([]string, error) {
	if agent == "" {
		return nil, errors.New("-agent is required")
	}
	t, err := readTable(filepath.Join(dir, agent, "tests.table"))
	if err != nil {
		return nil, err
	}
	old, err := filepath.Glob(filepath.Join(testsDir, "*"))
	if err != nil {
		return nil, err
	}
	for _, folder := range old {
		name := filepath.Base(folder)
		if strings.HasPrefix(name, "tool-") || strings.HasPrefix(name, "pair-") || strings.HasPrefix(name, "chain-") {
			if err := os.RemoveAll(folder); err != nil {
				return nil, err
			}
		}
	}

	var written []string
	write := func(name, about string, messages []string) error {
		folder := filepath.Join(testsDir, name)
		if err := writeTest(folder, agent, about, t, messages); err != nil {
			return err
		}
		written = append(written, folder)
		return nil
	}
	for _, tool := range t.tools {
		if err := write("tool-"+tool, "the model calls "+tool+" once per message, with the cases of the table", t.cases[tool]); err != nil {
			return nil, err
		}
	}
	for _, a := range t.pairs {
		for _, b := range t.pairs {
			message := fmt.Sprintf("Pair %s then %s [[tool %s %s]] [[then]] [[tool %s %s]]", a.tool, b.tool, a.tool, a.args, b.tool, b.args)
			about := "in one turn the model calls " + a.tool + ", then " + b.tool + " after the first result"
			if err := write("pair-"+a.tool+"-then-"+b.tool, about, []string{message}); err != nil {
				return nil, err
			}
		}
	}
	for _, name := range t.order {
		if err := write("chain-"+name, "the chain "+name+" of the table", t.chains[name]); err != nil {
			return nil, err
		}
	}
	return written, nil
}

func readTable(path string) (*table, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	t := &table{files: map[string]string{}, cases: map[string][]string{}, chains: map[string][]string{}}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for scanner.Scan() {
		line++
		text := scanner.Text()
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Split(text, "\t")
		bad := fmt.Errorf("%s:%d: cannot read %q", path, line, text)
		switch {
		case fields[0] == "inbound" && len(fields) >= 2:
			t.inbound = fields[1:]
		case fields[0] == "file" && len(fields) == 3:
			t.files[fields[1]] = unescape(fields[2])
		case fields[0] == "pair" && len(fields) == 3:
			t.pairs = append(t.pairs, pairCall{tool: fields[1], args: fields[2]})
		case fields[0] == "case" && len(fields) == 3:
			if _, seen := t.cases[fields[1]]; !seen {
				t.tools = append(t.tools, fields[1])
			}
			t.cases[fields[1]] = append(t.cases[fields[1]], fields[2])
		case fields[0] == "chain" && len(fields) == 3:
			if _, seen := t.chains[fields[1]]; !seen {
				t.order = append(t.order, fields[1])
			}
			t.chains[fields[1]] = append(t.chains[fields[1]], fields[2])
		default:
			return nil, bad
		}
	}
	return t, scanner.Err()
}

// unescape undoes the \n, \t and \\ escapes of the table.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func writeTest(folder, agent, about string, t *table, messages []string) error {
	if len(messages) == 0 {
		return fmt.Errorf("%s has no messages", folder)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	var inbound strings.Builder
	for i, message := range messages {
		for _, field := range t.inbound {
			inbound.WriteString(strings.ReplaceAll(field, "{uptime}", strconv.Itoa(1000*(i+1))))
			inbound.WriteByte('\t')
		}
		inbound.WriteString(message)
		inbound.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(folder, "inbound.txt"), []byte(inbound.String()), 0o644); err != nil {
		return err
	}
	names := make([]string, 0, len(t.files))
	for name := range t.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(folder, "spiffs", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(t.files[name]), 0o644); err != nil {
			return err
		}
	}
	script := fmt.Sprintf("#!/bin/sh\n# Generated by testgen from ../../tests.table: %s.\n"+
		"exec \"$(dirname \"$0\")/../../../testlib/tool-test.sh\" %s \"$(dirname \"$0\")\"\n", about, agent)
	return os.WriteFile(filepath.Join(folder, "run.sh"), []byte(script), 0o755)
}

func generatedName(name string) bool {
	return strings.HasPrefix(name, "tool-") || strings.HasPrefix(name, "pair-") || strings.HasPrefix(name, "chain-")
}

// recorded is what recording adds to a folder.
var recorded = map[string]bool{"tape.txt": true, "expected-output.txt": true}

// sameInputs reports whether the committed folder holds exactly the inputs
// of the generated one, besides its recording.
func sameInputs(generated, committed string) bool {
	files := map[string]bool{}
	err := filepath.WalkDir(committed, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(committed, path)
		if err == nil && !recorded[rel] {
			files[rel] = true
		}
		return err
	})
	if err != nil {
		return false
	}
	err = filepath.WalkDir(generated, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(generated, path)
		if err != nil {
			return err
		}
		want, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		have, err := os.ReadFile(filepath.Join(committed, rel))
		if err != nil || string(have) != string(want) {
			return errors.New("differs")
		}
		delete(files, rel)
		return nil
	})
	return err == nil && len(files) == 0
}

// compare generates the folders of agent into a temporary folder and calls
// found for each with whether the committed one has the same inputs. It
// returns the committed generated folders the table no longer has.
func compare(dir, agent string, found func(name, generated string, same bool) error) ([]string, int, error) {
	tmp, err := os.MkdirTemp("", "testgen-")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(tmp)
	written, err := generate(dir, agent, tmp)
	if err != nil {
		return nil, 0, err
	}
	committed := filepath.Join(dir, agent, "tests")
	want := map[string]bool{}
	for _, folder := range written {
		name := filepath.Base(folder)
		want[name] = true
		if err := found(name, folder, sameInputs(folder, filepath.Join(committed, name))); err != nil {
			return nil, 0, err
		}
	}
	entries, err := os.ReadDir(committed)
	if err != nil {
		return nil, 0, err
	}
	var stale []string
	for _, e := range entries {
		if generatedName(e.Name()) && !want[e.Name()] {
			stale = append(stale, filepath.Join(committed, e.Name()))
		}
	}
	return stale, len(written), nil
}

// checkFolders fails when a committed folder is missing, extra, has other
// inputs than the table gives, or has no recording.
func checkFolders(dir, agent string) error {
	committed := filepath.Join(dir, agent, "tests")
	stale, count, err := compare(dir, agent, func(name, _ string, same bool) error {
		if !same {
			return fmt.Errorf("%s/tests/%s differs from the table, run testgen -agent %s and record it", agent, name, agent)
		}
		if _, err := os.Stat(filepath.Join(committed, name, "tape.txt")); err != nil {
			return fmt.Errorf("%s/tests/%s is not recorded", agent, name)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(stale) > 0 {
		return fmt.Errorf("%s is not in the table", stale[0])
	}
	fmt.Printf("%s: %d generated folders match tests.table\n", agent, count)
	return nil
}

// update writes the folders whose inputs changed, removes the ones the
// table no longer has, and prints the folders that need recording.
func update(dir, agent string) error {
	committed := filepath.Join(dir, agent, "tests")
	stale, _, err := compare(dir, agent, func(name, generated string, same bool) error {
		target := filepath.Join(committed, name)
		if !same {
			if err := os.RemoveAll(target); err != nil {
				return err
			}
			if err := copyTree(generated, target); err != nil {
				return err
			}
		}
		if _, err := os.Stat(filepath.Join(target, "tape.txt")); err != nil {
			fmt.Println(target)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, folder := range stale {
		if err := os.RemoveAll(folder); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, info.Mode().Perm())
	})
}
