package main

import (
	"fmt"
	"strings"

	"github.com/skycoin/skycoin-cx-playground-android/testreport"
)

// resultsFile holds a test's result lines in its results directory, and
// the suite report repeats them.
const resultsFile = "results.txt"

// programResults returns the result lines of the test programs among the
// console output in the trace lines, see testreport.
func programResults(traces []string) ([]testreport.Result, error) {
	var out []string
	for _, l := range traces {
		if text, ok := strings.CutPrefix(l, "out "); ok {
			out = append(out, text)
		}
	}
	return testreport.ParseOutput(out)
}

// checkResults returns the failures of the result and results-pass lines
// of expect against the results of the run.
func checkResults(expect []expectation, results []testreport.Result) []string {
	var failures []string
	for _, e := range expect {
		switch e.verb {
		case "result":
			found := false
			for _, r := range results {
				if r.Name != e.args[0] {
					continue
				}
				found = true
				if string(r.Verdict) != e.args[1] {
					failures = append(failures, fmt.Sprintf("%s: got %s", e.source, r))
				}
			}
			if !found {
				failures = append(failures, fmt.Sprintf("%s: no result line", e.source))
			}
		case "results-pass":
			if len(results) == 0 {
				failures = append(failures, e.source+": no result lines")
			}
			for _, r := range results {
				if !r.Passed() {
					failures = append(failures, fmt.Sprintf("%s: %s", e.source, r))
				}
			}
		}
	}
	return failures
}

// resultsText is the result lines, one per line, for results.txt.
func resultsText(results []testreport.Result) string {
	var b strings.Builder
	for _, r := range results {
		b.WriteString(r.String())
		b.WriteByte('\n')
	}
	return b.String()
}
