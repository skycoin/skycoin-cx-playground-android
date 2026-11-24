package benchsuite

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/textform"
)

// SchemaName is the schema field of every results document this version
// writes and the collector accepts.
const SchemaName = "cx-playground-benchmark/2"

// MaxDocumentBytes bounds a results document. A full run is about 6 KiB.
const MaxDocumentBytes = 1 << 20

// Result statuses.
const (
	StatusPass     = "pass"     // printed the expected hash and the pass line
	StatusFail     = "fail"     // ran, but printed another hash or no pass line
	StatusRejected = "rejected" // the vm does not accept the program
	StatusError    = "error"    // compile or runtime error
)

// ResultsDocument is the document of one suite run, in the text form.
type ResultsDocument struct {
	Schema string
	// RunID is 16 random bytes in hex, the only identifier in the document.
	RunID      string
	StartedAt  time.Time
	FinishedAt time.Time
	Build      AppBuild
	Device     DeviceProfile
	Suite      SuiteSettings
	Results    []BenchmarkResult
}

// AppBuild says what ran the suite.
type AppBuild struct {
	AppVersion string
	// AppRevision is the vcs revision Go stamped into the binary, if any.
	AppRevision string
	// SkycoinCx is the skycoin-cx module version, its commit at the end.
	SkycoinCx string
	GoVersion string
}

// DeviceProfile describes the hardware and OS. It holds no personal
// identifier: no serial, IMEI, account or location.
type DeviceProfile struct {
	OS             string
	Arch           string
	AndroidRelease string
	AndroidSdk     int
	Manufacturer   string
	Model          string
	Abi            string
	CPUCores       int
	// CPUMaxMHz is the highest cpuinfo_max_freq of any core, 0 if unreadable.
	CPUMaxMHz  int
	RAMTotalMB int
}

// SuiteSettings are the scale and repetition of the run.
type SuiteSettings struct {
	Scale string
	Sizes Sizes
	Runs  int
}

// BenchmarkResult is one program on one executor.
type BenchmarkResult struct {
	Benchmark string
	Executor  string
	Status    string
	// RunsMs are the wall times of main in milliseconds, compile excluded.
	RunsMs       []float64
	MedianMs     float64
	Hash         string
	ExpectedHash string
	Operations   uint64
	OpsPerMs     float64
	Message      string
}

// resultColumns names the columns of the [results] table. The message is
// last, so it may hold commas.
const resultColumns = "benchmark, executor, status, median_ms, hash, expected_hash, operations, ops_per_ms, runs_ms, message"

func formatFloat(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

// EncodeResultsDocument returns doc in the text form (#38): [run], [build],
// [device] and [suite] sections of key = value lines and a [results] table
// with one result per line. runs_ms is space separated.
func EncodeResultsDocument(doc *ResultsDocument) ([]byte, error) {
	var w textform.Writer
	w.Comment("CX playground benchmark run, text form.")
	w.Section("run")
	w.Key("schema", doc.Schema)
	w.Key("run_id", doc.RunID)
	w.Key("started_at", doc.StartedAt.Format(time.RFC3339Nano))
	w.Key("finished_at", doc.FinishedAt.Format(time.RFC3339Nano))
	w.Section("build")
	for _, kv := range doc.Build.keys() {
		w.Key(kv[0], kv[1])
	}
	w.Section("device")
	for _, kv := range doc.Device.keys() {
		w.Key(kv[0], kv[1])
	}
	w.Section("suite")
	w.Key("scale", doc.Suite.Scale)
	w.Key("runs", strconv.Itoa(doc.Suite.Runs))
	sizes := doc.Suite.Sizes
	for _, name := range sizeNames {
		w.Key(name, fmt.Sprint(reflectValue(sizeFields(&sizes)[name])))
	}
	w.Section("results")
	w.Comment(resultColumns)
	for _, r := range doc.Results {
		runs := make([]string, len(r.RunsMs))
		for i, v := range r.RunsMs {
			runs[i] = formatFloat(v)
		}
		if strings.ContainsAny(r.Benchmark+r.Executor+r.Status+r.Hash+r.ExpectedHash, ",\n") {
			return nil, fmt.Errorf("result %s %s: a comma in a column", r.Benchmark, r.Executor)
		}
		w.Row(r.Benchmark, r.Executor, r.Status, formatFloat(r.MedianMs), r.Hash, r.ExpectedHash,
			strconv.FormatUint(r.Operations, 10), formatFloat(r.OpsPerMs), strings.Join(runs, " "), r.Message)
	}
	return w.Bytes(), nil
}

// sizeNames are the [sizes] keys in their order.
var sizeNames = []string{"count_n", "fib_n", "array_rounds", "struct_rounds", "mem_n", "mem_walk", "sieve_n", "miller_n", "lucas_rounds"}

func reflectValue(p any) any {
	switch v := p.(type) {
	case *int:
		return *v
	case *int64:
		return *v
	case *uint64:
		return *v
	}
	return p
}

func (b AppBuild) keys() [][2]string {
	out := [][2]string{{"app_version", b.AppVersion}}
	if b.AppRevision != "" {
		out = append(out, [2]string{"app_revision", b.AppRevision})
	}
	return append(out, [2]string{"skycoin_cx", b.SkycoinCx}, [2]string{"go_version", b.GoVersion})
}

func (d DeviceProfile) keys() [][2]string {
	out := [][2]string{{"os", d.OS}, {"arch", d.Arch}}
	for _, kv := range [][2]string{{"android_release", d.AndroidRelease}, {"manufacturer", d.Manufacturer}, {"model", d.Model}, {"abi", d.Abi}} {
		if kv[1] != "" {
			out = append(out, kv)
		}
	}
	if d.AndroidSdk != 0 {
		out = append(out, [2]string{"android_sdk", strconv.Itoa(d.AndroidSdk)})
	}
	return append(out, [2]string{"cpu_cores", strconv.Itoa(d.CPUCores)}, [2]string{"cpu_max_mhz", strconv.Itoa(d.CPUMaxMHz)},
		[2]string{"ram_total_mb", strconv.Itoa(d.RAMTotalMB)})
}

// DecodeResultsDocument reads one results document in the text form,
// rejecting unknown sections and keys, a section given twice, and
// documents that fail Validate.
func DecodeResultsDocument(r io.Reader) (*ResultsDocument, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxDocumentBytes {
		return nil, errors.New("document larger than 1 MiB")
	}
	doc, err := parseResultsDocument(string(b))
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return doc, nil
}

func parseResultsDocument(text string) (*ResultsDocument, error) {
	d, err := textform.Parse(text)
	if err != nil {
		return nil, err
	}
	if err := d.Only("run", "build", "device", "suite", "results"); err != nil {
		return nil, err
	}
	doc := &ResultsDocument{}
	section := func(name string, keys ...string) (map[string]string, error) {
		s, _ := d.Section(name)
		return s.Keys(keys...)
	}
	k, err := section("run", "schema", "run_id", "started_at", "finished_at")
	if err != nil {
		return nil, err
	}
	doc.Schema, doc.RunID = k["schema"], k["run_id"]
	for _, t := range []struct {
		key string
		to  *time.Time
	}{{"started_at", &doc.StartedAt}, {"finished_at", &doc.FinishedAt}} {
		if k[t.key] == "" {
			continue
		}
		if *t.to, err = time.Parse(time.RFC3339Nano, k[t.key]); err != nil {
			return nil, fmt.Errorf("%s: %w", t.key, err)
		}
	}
	if k, err = section("build", "app_version", "app_revision", "skycoin_cx", "go_version"); err != nil {
		return nil, err
	}
	doc.Build = AppBuild{AppVersion: k["app_version"], AppRevision: k["app_revision"], SkycoinCx: k["skycoin_cx"], GoVersion: k["go_version"]}
	if k, err = section("device", "os", "arch", "android_release", "android_sdk", "manufacturer", "model", "abi", "cpu_cores", "cpu_max_mhz", "ram_total_mb"); err != nil {
		return nil, err
	}
	doc.Device = DeviceProfile{OS: k["os"], Arch: k["arch"], AndroidRelease: k["android_release"], Manufacturer: k["manufacturer"], Model: k["model"], Abi: k["abi"]}
	for _, n := range []struct {
		key string
		to  *int
	}{{"android_sdk", &doc.Device.AndroidSdk}, {"cpu_cores", &doc.Device.CPUCores}, {"cpu_max_mhz", &doc.Device.CPUMaxMHz}, {"ram_total_mb", &doc.Device.RAMTotalMB}} {
		if k[n.key] == "" {
			continue
		}
		if *n.to, err = strconv.Atoi(k[n.key]); err != nil {
			return nil, fmt.Errorf("device %s: %w", n.key, err)
		}
	}
	if k, err = section("suite", append([]string{"scale", "runs"}, sizeNames...)...); err != nil {
		return nil, err
	}
	doc.Suite.Scale = k["scale"]
	if k["runs"] != "" {
		if doc.Suite.Runs, err = strconv.Atoi(k["runs"]); err != nil {
			return nil, fmt.Errorf("suite runs: %w", err)
		}
	}
	fields := sizeFields(&doc.Suite.Sizes)
	for _, name := range sizeNames {
		if k[name] == "" {
			continue
		}
		if err := setNumber(fields[name], k[name]); err != nil {
			return nil, fmt.Errorf("suite %s: %w", name, err)
		}
	}
	s, _ := d.Section("results")
	rows, err := s.Rows(10)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		r := BenchmarkResult{Benchmark: row[0], Executor: row[1], Status: row[2], Hash: row[4], ExpectedHash: row[5], Message: row[9], RunsMs: []float64{}}
		if r.MedianMs, err = strconv.ParseFloat(row[3], 64); err != nil {
			return nil, fmt.Errorf("result %s %s median_ms: %w", r.Benchmark, r.Executor, err)
		}
		if r.Operations, err = strconv.ParseUint(row[6], 10, 64); err != nil {
			return nil, fmt.Errorf("result %s %s operations: %w", r.Benchmark, r.Executor, err)
		}
		if r.OpsPerMs, err = strconv.ParseFloat(row[7], 64); err != nil {
			return nil, fmt.Errorf("result %s %s ops_per_ms: %w", r.Benchmark, r.Executor, err)
		}
		for _, f := range strings.Fields(row[8]) {
			v, err := strconv.ParseFloat(f, 64)
			if err != nil {
				return nil, fmt.Errorf("result %s %s runs_ms: %w", r.Benchmark, r.Executor, err)
			}
			r.RunsMs = append(r.RunsMs, v)
		}
		doc.Results = append(doc.Results, r)
	}
	return doc, nil
}

func validHex(s string, size int) bool {
	if len(s) != size {
		return false
	}
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f') {
			return false
		}
	}
	return true
}

// Validate checks the fields a collector relies on.
func (doc *ResultsDocument) Validate() error {
	switch {
	case doc.Schema != SchemaName:
		return fmt.Errorf("schema %q, want %q", doc.Schema, SchemaName)
	case !validHex(doc.RunID, 32):
		return errors.New("run_id must be 32 lowercase hex digits")
	case doc.StartedAt.IsZero() || doc.FinishedAt.Before(doc.StartedAt):
		return errors.New("started_at and finished_at must be set, in order")
	case doc.Build.AppVersion == "" || doc.Build.GoVersion == "":
		return errors.New("build.app_version and build.go_version are required")
	case doc.Device.OS == "" || doc.Device.Arch == "" || doc.Device.CPUCores < 1:
		return errors.New("device.os, device.arch and device.cpu_cores are required")
	case doc.Suite.Scale == "" || doc.Suite.Runs < 1:
		return errors.New("suite.scale and suite.runs are required")
	case len(doc.Results) == 0:
		return errors.New("no results")
	}
	for i, r := range doc.Results {
		where := fmt.Sprintf("results[%d]", i)
		if !slices.Contains(BenchmarkNames, r.Benchmark) {
			return fmt.Errorf("%s: unknown benchmark %q", where, r.Benchmark)
		}
		if r.Executor != "old" && r.Executor != "vm" {
			return fmt.Errorf("%s: unknown executor %q", where, r.Executor)
		}
		if _, err := strconv.ParseUint(r.ExpectedHash, 10, 64); err != nil {
			return fmt.Errorf("%s: expected_hash: %v", where, err)
		}
		switch r.Status {
		case StatusPass, StatusFail:
			if len(r.RunsMs) != doc.Suite.Runs || r.MedianMs < 0 {
				return fmt.Errorf("%s: %d runs_ms, want %d", where, len(r.RunsMs), doc.Suite.Runs)
			}
			if r.Status == StatusPass && r.Hash != r.ExpectedHash {
				return fmt.Errorf("%s: pass with hash %q, expected %q", where, r.Hash, r.ExpectedHash)
			}
		case StatusRejected, StatusError:
			if r.Message == "" {
				return fmt.Errorf("%s: %s without a message", where, r.Status)
			}
		default:
			return fmt.Errorf("%s: unknown status %q", where, r.Status)
		}
	}
	return nil
}
