// Package resultsschemaroundtrip checks that a results document survives
// encoding and decoding and that malformed documents are refused.
package resultsschemaroundtrip

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/benchsuite"
)

func sampleDocument() *benchsuite.ResultsDocument {
	started := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	return &benchsuite.ResultsDocument{
		Schema:     benchsuite.SchemaName,
		RunID:      "0123456789abcdef0123456789abcdef",
		StartedAt:  started,
		FinishedAt: started.Add(90 * time.Second),
		Build:      benchsuite.CollectAppBuild(),
		Device:     benchsuite.CollectDeviceProfile(),
		Suite:      benchsuite.SuiteSettings{Scale: "phone", Sizes: benchsuite.Sizes{CountN: 1000000, FibN: 27}, Runs: 3},
		Results: []benchsuite.BenchmarkResult{
			{Benchmark: "count", Executor: "old", Status: benchsuite.StatusPass, RunsMs: []float64{900.5, 899.25, 901}, MedianMs: 900.5,
				Hash: "17727803403943808447", ExpectedHash: "17727803403943808447", Operations: 6000014, OpsPerMs: 6662.9},
			{Benchmark: "fib", Executor: "vm", Status: benchsuite.StatusRejected, RunsMs: []float64{},
				ExpectedHash: "12546183366041830149", Message: "vm does not accept the program: example"},
		},
	}
}

func TestRoundTrip(t *testing.T) {
	doc := sampleDocument()
	b, err := benchsuite.EncodeResultsDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	back, err := benchsuite.DecodeResultsDocument(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc, back) {
		t.Fatalf("round trip changed the document:\n%+v\n%+v", doc, back)
	}
	again, _ := benchsuite.EncodeResultsDocument(back)
	if !bytes.Equal(b, again) {
		t.Fatal("encoding is not stable")
	}
}

func TestRejects(t *testing.T) {
	for name, c := range map[string]struct {
		mutate func(*benchsuite.ResultsDocument)
		want   string
	}{
		"schema":          {func(d *benchsuite.ResultsDocument) { d.Schema = "other/1" }, "schema"},
		"run id":          {func(d *benchsuite.ResultsDocument) { d.RunID = "device-serial-123" }, "run_id"},
		"no results":      {func(d *benchsuite.ResultsDocument) { d.Results = nil }, "no results"},
		"benchmark":       {func(d *benchsuite.ResultsDocument) { d.Results[0].Benchmark = "nbody" }, "unknown benchmark"},
		"executor":        {func(d *benchsuite.ResultsDocument) { d.Results[0].Executor = "jit" }, "unknown executor"},
		"runs count":      {func(d *benchsuite.ResultsDocument) { d.Results[0].RunsMs = d.Results[0].RunsMs[:1] }, "runs_ms"},
		"pass wrong hash": {func(d *benchsuite.ResultsDocument) { d.Results[0].Hash = "1" }, "pass with hash"},
		"no message":      {func(d *benchsuite.ResultsDocument) { d.Results[1].Message = "" }, "without a message"},
		"finished early":  {func(d *benchsuite.ResultsDocument) { d.FinishedAt = d.StartedAt.Add(-time.Second) }, "in order"},
	} {
		doc := sampleDocument()
		c.mutate(doc)
		b, _ := benchsuite.EncodeResultsDocument(doc)
		if _, err := benchsuite.DecodeResultsDocument(bytes.NewReader(b)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want %q", name, err, c.want)
		}
	}
	b, _ := benchsuite.EncodeResultsDocument(sampleDocument())
	extra := bytes.Replace(b, []byte("schema = "), []byte("imei = 1\nschema = "), 1)
	if _, err := benchsuite.DecodeResultsDocument(bytes.NewReader(extra)); err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Errorf("unknown key: err %v", err)
	}
	if _, err := benchsuite.DecodeResultsDocument(bytes.NewReader(append(b, b...))); err == nil {
		t.Error("two documents accepted")
	}
}
