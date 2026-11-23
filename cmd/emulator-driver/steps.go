package main

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// inputStep is one line of a test's input.txt:
//
//	text <string>              type text (adb shell input text)
//	key <NAME or code>         press a key, ENTER or KEYCODE_ENTER or 66
//	tap <x> <y>                tap at a screen pixel
//	swipe <x1> <y1> <x2> <y2> [ms]
//	screenshot <name>          save the screen as <name>.png
//	wait <ms>                  pause
type inputStep struct {
	verb string
	args []string
	line string
}

// injected reports whether the step sends an input event to the app.
func (s inputStep) injected() bool {
	switch s.verb {
	case "text", "key", "tap", "swipe":
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
	}
	return nil
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
		want := map[string][2]int{"key": {1, 1}, "tap": {2, 2}, "swipe": {4, 5}, "screenshot": {1, 1}, "wait": {1, 1}}
		switch verb {
		case "text":
			if rest == "" {
				return nil, fmt.Errorf("line %d: text needs a string", n)
			}
			s.args = []string{rest}
		case "key", "tap", "swipe", "screenshot", "wait":
			s.args = strings.Fields(rest)
			if c := want[verb]; len(s.args) < c[0] || len(s.args) > c[1] {
				return nil, fmt.Errorf("line %d: %q has the wrong number of arguments", n, line)
			}
			if verb == "tap" || verb == "swipe" || verb == "wait" {
				for _, a := range s.args {
					if _, err := strconv.Atoi(a); err != nil {
						return nil, fmt.Errorf("line %d: %q is not a number", n, a)
					}
				}
			}
		default:
			return nil, fmt.Errorf("line %d: unknown step %q", n, verb)
		}
		steps = append(steps, s)
	}
	return steps, sc.Err()
}

// expectation is one line of a test's expect.txt. A plain line must appear
// in the app's test mode output, after the line matched before it.
//
//	screen-changed <a> <b>     screenshots a and b differ
//	screen-matches <name>      screenshot name matches expected/<name>.png
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
		switch f[0] {
		case "screen-changed":
			if len(f) != 3 {
				return nil, fmt.Errorf("line %d: screen-changed takes two screenshot names", n)
			}
			out = append(out, expectation{verb: f[0], args: f[1:], source: line})
		case "screen-matches":
			if len(f) != 2 {
				return nil, fmt.Errorf("line %d: screen-matches takes one screenshot name", n)
			}
			out = append(out, expectation{verb: f[0], args: f[1:], source: line})
		default:
			out = append(out, expectation{text: line, source: line})
		}
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
