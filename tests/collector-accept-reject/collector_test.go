// Package collectoracceptreject posts documents to the benchmark collector
// and checks what it stores and lists.
package collectoracceptreject

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/benchcollector"
	"github.com/skycoin/skycoin-cx-playground-android/benchsuite"
)

func validDocument() *benchsuite.ResultsDocument {
	started := time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)
	return &benchsuite.ResultsDocument{
		Schema: benchsuite.SchemaName, RunID: "fedcba9876543210fedcba9876543210",
		StartedAt: started, FinishedAt: started.Add(time.Minute),
		Build:  benchsuite.AppBuild{AppVersion: "0.2.0", SkycoinCx: "v0.0.0-20261123002813-df4c987910ae", GoVersion: "go1.27.3"},
		Device: benchsuite.DeviceProfile{OS: "android", Arch: "arm64", AndroidRelease: "14", AndroidSdk: 34, Manufacturer: "Example", Model: "Phone 1", Abi: "arm64-v8a", CPUCores: 8, CPUMaxMHz: 2800, RAMTotalMB: 7600},
		Suite:  benchsuite.SuiteSettings{Scale: "phone", Runs: 1},
		Results: []benchsuite.BenchmarkResult{{Benchmark: "fib", Executor: "vm", Status: benchsuite.StatusPass, RunsMs: []float64{12.5}, MedianMs: 12.5,
			Hash: "12546183366041830149", ExpectedHash: "12546183366041830149"}},
	}
}

func post(t *testing.T, url string, body []byte) (int, string) {
	resp, err := http.Post(url+"/runs", "text/plain; charset=utf-8", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func get(t *testing.T, url string) string {
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestCollectorAcceptsAndRejects(t *testing.T) {
	runs, err := benchcollector.OpenRunStore(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer runs.Close()
	server := httptest.NewServer(&benchcollector.CollectorServer{Runs: runs})
	defer server.Close()

	doc := validDocument()
	reply, err := benchsuite.UploadResults(context.Background(), server.URL+"/runs", doc)
	if err != nil || !strings.Contains(reply, doc.RunID) {
		t.Fatalf("upload: %q %v", reply, err)
	}
	valid, _ := benchsuite.EncodeResultsDocument(doc)
	if code, body := post(t, server.URL, valid); code != http.StatusConflict {
		t.Errorf("duplicate run: %d %s", code, body)
	}
	bad := validDocument()
	bad.RunID, bad.Results[0].Executor = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "jit"
	badBytes, _ := benchsuite.EncodeResultsDocument(bad)
	for name, body := range map[string][]byte{
		"not text form": []byte("hello"),
		"bad executor":  badBytes,
		"unknown key":   bytes.Replace(valid, []byte("schema = "), []byte("serial = x\nschema = "), 1),
		"too large":     bytes.Repeat([]byte(" "), benchsuite.MaxDocumentBytes+10),
	} {
		if code, reply := post(t, server.URL, body); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, reply)
		}
	}

	stored, err := runs.ListRuns()
	if err != nil || len(stored) != 1 || stored[0].RunID != doc.RunID {
		t.Fatalf("stored %d runs, %v", len(stored), err)
	}
	table := get(t, server.URL+"/runs.txt")
	if !strings.Contains(table, doc.RunID) || !strings.Contains(table, "Example Phone 1") || !strings.Contains(table, "12.5") {
		t.Errorf("plain table:\n%s", table)
	}
	if page := get(t, server.URL+"/"); !strings.Contains(page, "<td>Example Phone 1</td>") {
		t.Errorf("html table:\n%s", page)
	}
	t.Logf("runs.txt:\n%s", table)
}
