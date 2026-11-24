// Package testreport reads and writes the result lines of the test programs
// of skycoin-cx-playground-android #19. Every test prints one line:
//
//	name verdict blocks_per_s kb_per_s latency_us [serial_read_kb_s serial_write_kb_s] [note]
//
// The verdict is true, false or skipped. A measurement is a decimal number,
// or - when the test did not measure it. The two serial speeds are present
// exactly when the name starts with Serial. The note is free text to the
// end of the line, for example "block=512B blocks=64 mismatches=0".
package testreport

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Verdict is the outcome of one test.
type Verdict string

// The verdicts.
const (
	VerdictTrue    Verdict = "true"
	VerdictFalse   Verdict = "false"
	VerdictSkipped Verdict = "skipped"
)

func (v Verdict) valid() bool {
	return v == VerdictTrue || v == VerdictFalse || v == VerdictSkipped
}

// SerialPrefix starts the names of the tests that report serial speeds.
const SerialPrefix = "Serial"

// NotMeasured is the text of a measurement the test did not make.
const NotMeasured = "-"

// Measurement is a number of a result line, kept as its text so a line
// reads back unchanged.
type Measurement string

// Value returns the number, and false when it was not measured.
func (m Measurement) Value() (float64, bool) {
	if m == NotMeasured {
		return 0, false
	}
	v, err := strconv.ParseFloat(string(m), 64)
	return v, err == nil
}

func (m Measurement) valid() bool {
	if m == NotMeasured {
		return true
	}
	v, err := strconv.ParseFloat(string(m), 64)
	return err == nil && v >= 0 && !strings.ContainsAny(string(m), "eEnNiI+")
}

// Result is one result line.
type Result struct {
	Name                   string
	Verdict                Verdict
	BlocksPerSecond        Measurement
	KBPerSecond            Measurement
	LatencyMicroseconds    Measurement
	SerialReadKBPerSecond  Measurement // only for Serial tests
	SerialWriteKBPerSecond Measurement // only for Serial tests
	Note                   string
}

// Serial reports whether the result carries the serial speeds.
func (r Result) Serial() bool { return strings.HasPrefix(r.Name, SerialPrefix) }

// Passed reports whether the result does not fail the run: true, or skipped.
func (r Result) Passed() bool { return r.Verdict == VerdictTrue || r.Verdict == VerdictSkipped }

// String returns the result line.
func (r Result) String() string {
	f := []string{r.Name, string(r.Verdict), string(r.BlocksPerSecond), string(r.KBPerSecond), string(r.LatencyMicroseconds)}
	if r.Serial() {
		f = append(f, string(r.SerialReadKBPerSecond), string(r.SerialWriteKBPerSecond))
	}
	if r.Note != "" {
		f = append(f, r.Note)
	}
	return strings.Join(f, " ")
}

// ErrNotResult is returned for a line whose second field is not a verdict,
// such as other output of a test program.
var ErrNotResult = errors.New("not a result line")

// ParseLine parses one result line.
func ParseLine(line string) (Result, error) {
	f := strings.Fields(line)
	if len(f) < 2 || !Verdict(f[1]).valid() {
		return Result{}, ErrNotResult
	}
	r := Result{Name: f[0], Verdict: Verdict(f[1])}
	if !validName(r.Name) {
		return r, fmt.Errorf("result name %q is not letters and digits starting with a capital", r.Name)
	}
	numbers := []*Measurement{&r.BlocksPerSecond, &r.KBPerSecond, &r.LatencyMicroseconds}
	if r.Serial() {
		numbers = append(numbers, &r.SerialReadKBPerSecond, &r.SerialWriteKBPerSecond)
	}
	if len(f) < 2+len(numbers) {
		return r, fmt.Errorf("result %s has %d of its %d measurements", r.Name, len(f)-2, len(numbers))
	}
	for i, m := range numbers {
		*m = Measurement(f[2+i])
		if !m.valid() {
			return r, fmt.Errorf("result %s measurement %d is %q, not a number or %s", r.Name, i+1, f[2+i], NotMeasured)
		}
	}
	r.Note = strings.Join(f[2+len(numbers):], " ")
	return r, nil
}

func validName(name string) bool {
	if name == "" || name[0] < 'A' || name[0] > 'Z' {
		return false
	}
	for _, c := range name {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// ParseOutput returns the result lines of a program's output, skipping
// the other lines. A malformed result line is an error.
func ParseOutput(lines []string) ([]Result, error) {
	var results []Result
	var errs []error
	for _, l := range lines {
		r, err := ParseLine(l)
		switch {
		case errors.Is(err, ErrNotResult):
		case err != nil:
			errs = append(errs, err)
		default:
			results = append(results, r)
		}
	}
	return results, errors.Join(errs...)
}
