package main

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// inputStep is one line of a test's input.txt:
//
//	text <string>              type text (adb shell input text)
//	key <NAME or code>         press a key, ENTER or KEYCODE_ENTER or 66
//	tap <x> <y>                tap at a screen pixel
//	swipe <x1> <y1> <x2> <y2> [ms]
//	rotate <0..3>              turn the display, 1 is landscape
//	screenshot <name>          save the screen as <name>.png
//	wait <ms>                  pause
//	wait-for <text>            wait for a test mode line containing text
//	keyboard shown|hidden      wait until the soft keyboard is in that state
//	pull <path> <name>         save a file the app published in test mode as <name>
type inputStep struct {
	verb string
	args []string
	line string
}

// injected reports whether the step sends an input event to the app.
func (s inputStep) injected() bool {
	switch s.verb {
	case "text", "key", "tap", "swipe", "rotate":
		return true
	}
	return false
}

// inputCommand returns the arguments of adb shell for an event step.
func (s inputStep) inputCommand() []string {
	switch s.verb {
	case "text":
		// input text takes %s for a space.
		return []string{"input", "text", shellQuote(strings.ReplaceAll(s.args[0], " ", "%s"))}
	case "key":
		k := s.args[0]
		if _, err := strconv.Atoi(k); err != nil && !strings.HasPrefix(k, "KEYCODE_") {
			k = "KEYCODE_" + strings.ToUpper(k)
		}
		return []string{"input", "keyevent", k}
	case "tap", "swipe":
		return append([]string{"input", s.verb}, s.args...)
	case "rotate":
		// Works on the emulator and on a phone, unlike adb emu rotate.
		return []string{"settings", "put", "system", "accelerometer_rotation", "0", ";",
			"settings", "put", "system", "user_rotation", s.args[0]}
	}
	return nil
}

// stepArgs is the least and most number of arguments of each verb that
// takes space separated arguments, and whether they must be numbers.
var stepArgs = map[string]struct {
	least, most int
	numbers     bool
}{
	"key":        {1, 1, false},
	"tap":        {2, 2, true},
	"swipe":      {4, 5, true},
	"rotate":     {1, 1, true},
	"screenshot": {1, 1, false},
	"wait":       {1, 1, true},
	"keyboard":   {1, 1, false},
	"pull":       {2, 2, false},
}

func parseSteps(r io.Reader) ([]inputStep, error) {
	var steps []inputStep
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		verb, rest, _ := strings.Cut(line, " ")
		rest = strings.TrimSpace(rest)
		s := inputStep{verb: verb, line: line}
		switch c, ok := stepArgs[verb]; {
		case verb == "text" || verb == "wait-for":
			if rest == "" {
				return nil, fmt.Errorf("line %d: %s needs a string", n, verb)
			}
			s.args = []string{rest}
		case ok:
			s.args = strings.Fields(rest)
			if len(s.args) < c.least || len(s.args) > c.most {
				return nil, fmt.Errorf("line %d: %q has the wrong number of arguments", n, line)
			}
			for _, a := range s.args {
				if _, err := strconv.Atoi(a); c.numbers && err != nil {
					return nil, fmt.Errorf("line %d: %q is not a number", n, a)
				}
			}
		default:
			return nil, fmt.Errorf("line %d: unknown step %q", n, verb)
		}
		if verb == "rotate" && (s.args[0] < "0" || s.args[0] > "3") {
			return nil, fmt.Errorf("line %d: rotate takes 0, 1, 2 or 3", n)
		}
		if verb == "keyboard" && s.args[0] != "shown" && s.args[0] != "hidden" {
			return nil, fmt.Errorf("line %d: keyboard takes shown or hidden", n)
		}
		steps = append(steps, s)
	}
	return steps, sc.Err()
}

// expectation is one line of a test's expect.txt. A plain line must appear
// in the app's test mode output, after the line matched before it.
//
//	screen-changed <a> <b>             screenshots a and b differ
//	screen-matches <name> [x y w h]    screenshot name, or its region, matches expected/<name>.png
//	file-equals <pulled> <path>        a pulled file equals path, relative to the test folder
//	file-contains <pulled> <text>      a pulled file contains text
//	result <name> <verdict>            the test program printed result line name with verdict, see testreport
//	results-pass                       every result line printed is true or skipped
type expectation struct {
	text   string
	verb   string
	args   []string
	source string
}

func parseExpectations(r io.Reader) ([]expectation, error) {
	var out []expectation
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		e := expectation{verb: f[0], args: f[1:], source: line}
		switch f[0] {
		case "screen-changed":
			if len(f) != 3 {
				return nil, fmt.Errorf("line %d: screen-changed takes two screenshot names", n)
			}
		case "screen-matches":
			if len(f) != 2 && len(f) != 6 {
				return nil, fmt.Errorf("line %d: screen-matches takes a screenshot name and an optional x y w h region", n)
			}
			for _, a := range f[2:] {
				if v, err := strconv.Atoi(a); err != nil || v < 0 {
					return nil, fmt.Errorf("line %d: %q is not a pixel count", n, a)
				}
			}
		case "file-equals":
			if len(f) != 3 {
				return nil, fmt.Errorf("line %d: file-equals takes a pulled name and a path", n)
			}
		case "file-contains":
			name, text, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, f[0])), " ")
			if text = strings.TrimSpace(text); text == "" {
				return nil, fmt.Errorf("line %d: file-contains takes a pulled name and a text", n)
			}
			e.args = []string{name, text}
		case "result":
			if len(f) != 3 || !slices.Contains([]string{"true", "false", "skipped"}, f[2]) {
				return nil, fmt.Errorf("line %d: result takes a test name and true, false or skipped", n)
			}
		case "results-pass":
			if len(f) != 1 {
				return nil, fmt.Errorf("line %d: results-pass takes nothing", n)
			}
		default:
			e = expectation{text: line, source: line}
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// unmatchedText returns the first text expectation that the trace lines do
// not satisfy in order, or "" when all match.
func unmatchedText(expect []expectation, lines []string) string {
	i := 0
	for _, e := range expect {
		if e.verb != "" {
			continue
		}
		for i < len(lines) && !strings.Contains(lines[i], e.text) {
			i++
		}
		if i == len(lines) {
			return e.text
		}
		i++
	}
	return ""
}
