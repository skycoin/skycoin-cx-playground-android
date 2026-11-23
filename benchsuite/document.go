package benchsuite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"
)

// SchemaName is the schema field of every results document this version
// writes and the collector accepts.
const SchemaName = "cx-playground-benchmark/1"

// MaxDocumentBytes bounds a results document. A full run is about 6 KiB.
const MaxDocumentBytes = 1 << 20

// Result statuses.
const (
	StatusPass     = "pass"     // printed the expected hash and the pass line
	StatusFail     = "fail"     // ran, but printed another hash or no pass line
	StatusRejected = "rejected" // the vm does not accept the program
	StatusError    = "error"    // compile or runtime error
)

// ResultsDocument is the JSON document of one suite run.
type ResultsDocument struct {
	Schema string `json:"schema"`
	// RunID is 16 random bytes in hex, the only identifier in the document.
	RunID      string            `json:"run_id"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	Build      AppBuild          `json:"build"`
	Device     DeviceProfile     `json:"device"`
	Suite      SuiteSettings     `json:"suite"`
	Results    []BenchmarkResult `json:"results"`
}

// AppBuild says what ran the suite.
type AppBuild struct {
	AppVersion string `json:"app_version"`
	// AppRevision is the vcs revision Go stamped into the binary, if any.
	AppRevision string `json:"app_revision,omitempty"`
	// SkycoinCx is the skycoin-cx module version, its commit at the end.
	SkycoinCx string `json:"skycoin_cx"`
	GoVersion string `json:"go_version"`
}

// DeviceProfile describes the hardware and OS. It holds no personal
// identifier: no serial, IMEI, account or location.
type DeviceProfile struct {
	OS             string `json:"os"`
	Arch           string `json:"arch"`
	AndroidRelease string `json:"android_release,omitempty"`
	AndroidSdk     int    `json:"android_sdk,omitempty"`
	Manufacturer   string `json:"manufacturer,omitempty"`
	Model          string `json:"model,omitempty"`
	Abi            string `json:"abi,omitempty"`
	CPUCores       int    `json:"cpu_cores"`
	// CPUMaxMHz is the highest cpuinfo_max_freq of any core, 0 if unreadable.
	CPUMaxMHz  int `json:"cpu_max_mhz"`
	RAMTotalMB int `json:"ram_total_mb"`
}

// SuiteSettings are the scale and repetition of the run.
type SuiteSettings struct {
	Scale string `json:"scale"`
	Sizes Sizes  `json:"sizes"`
	Runs  int    `json:"runs"`
}

// BenchmarkResult is one program on one executor.
type BenchmarkResult struct {
	Benchmark string `json:"benchmark"`
	Executor  string `json:"executor"`
	Status    string `json:"status"`
	// RunsMs are the wall times of main in milliseconds, compile excluded.
	RunsMs       []float64 `json:"runs_ms"`
	MedianMs     float64   `json:"median_ms"`
	Hash         string    `json:"hash"`
	ExpectedHash string    `json:"expected_hash"`
	Operations   uint64    `json:"operations,omitempty"`
	OpsPerMs     float64   `json:"ops_per_ms,omitempty"`
	Message      string    `json:"message,omitempty"`
}

// EncodeResultsDocument returns doc as indented JSON.
func EncodeResultsDocument(doc *ResultsDocument) ([]byte, error) {
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// DecodeResultsDocument reads one results document, rejecting unknown
// fields, trailing data and documents that fail Validate.
func DecodeResultsDocument(r io.Reader) (*ResultsDocument, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxDocumentBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxDocumentBytes {
		return nil, errors.New("document larger than 1 MiB")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var doc ResultsDocument
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("data after the document")
	}
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	return &doc, nil
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
