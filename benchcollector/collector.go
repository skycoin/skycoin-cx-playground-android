// Package benchcollector accepts benchmark results documents over HTTP,
// stores them in a bbolt file and lists them as a table.
package benchcollector

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/skycoin/skycoin-cx-playground-android/benchsuite"
)

var runsBucket = []byte("runs")

// ErrDuplicateRun is returned for a run ID that is already stored.
var ErrDuplicateRun = errors.New("run already stored")

// RunStore keeps results documents in a bbolt file, keyed by run ID.
type RunStore struct {
	db *bolt.DB
}

// OpenRunStore opens or creates the bbolt file at path.
func OpenRunStore(path string) (*RunStore, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(runsBucket)
		return err
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &RunStore{db: db}, nil
}

// Close closes the file.
func (s *RunStore) Close() error { return s.db.Close() }

// StoreRun saves doc, which must already be valid.
func (s *RunStore) StoreRun(doc *benchsuite.ResultsDocument) error {
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(runsBucket)
		if bucket.Get([]byte(doc.RunID)) != nil {
			return ErrDuplicateRun
		}
		return bucket.Put([]byte(doc.RunID), b)
	})
}

// ListRuns returns every stored document, newest first.
func (s *RunStore) ListRuns() ([]*benchsuite.ResultsDocument, error) {
	var runs []*benchsuite.ResultsDocument
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(runsBucket).ForEach(func(_, v []byte) error {
			var doc benchsuite.ResultsDocument
			if err := json.Unmarshal(v, &doc); err != nil {
				return err
			}
			runs = append(runs, &doc)
			return nil
		})
	})
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
	return runs, err
}

// CollectorServer serves POST /runs, GET /runs.txt and GET / (HTML).
type CollectorServer struct {
	Runs *RunStore
}

func (c *CollectorServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/runs":
		c.acceptRun(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/runs.txt":
		c.writeRunTable(w, false)
	case r.Method == http.MethodGet && r.URL.Path == "/":
		c.writeRunTable(w, true)
	default:
		http.Error(w, "POST /runs, GET /runs.txt or GET /", http.StatusNotFound)
	}
}

func (c *CollectorServer) acceptRun(w http.ResponseWriter, r *http.Request) {
	doc, err := benchsuite.DecodeResultsDocument(http.MaxBytesReader(w, r.Body, benchsuite.MaxDocumentBytes))
	if err != nil {
		http.Error(w, "invalid document: "+err.Error(), http.StatusBadRequest)
		return
	}
	switch err := c.Runs.StoreRun(doc); {
	case errors.Is(err, ErrDuplicateRun):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, "stored %s\n", doc.RunID)
	}
}

// runTableHeader names the columns of the run table, one row per result.
var runTableHeader = []string{"started", "run", "device", "android", "cores", "mhz", "scale", "bench", "exec", "status", "median_ms", "ops/ms"}

func runTableRows(runs []*benchsuite.ResultsDocument) [][]string {
	var rows [][]string
	for _, doc := range runs {
		device := strings.TrimSpace(doc.Device.Manufacturer + " " + doc.Device.Model)
		if device == "" {
			device = doc.Device.OS + "/" + doc.Device.Arch
		}
		for _, res := range doc.Results {
			rows = append(rows, cleanCells(
				doc.StartedAt.Format("2006-01-02 15:04"), doc.RunID, device,
				fmt.Sprint(doc.Device.AndroidRelease), fmt.Sprint(doc.Device.CPUCores), fmt.Sprint(doc.Device.CPUMaxMHz),
				doc.Suite.Scale, res.Benchmark, res.Executor, res.Status,
				fmt.Sprintf("%.1f", res.MedianMs), fmt.Sprintf("%.0f", res.OpsPerMs),
			))
		}
	}
	return rows
}

func (c *CollectorServer) writeRunTable(w http.ResponseWriter, htmlPage bool) {
	runs, err := c.Runs.ListRuns()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows := runTableRows(runs)
	if !htmlPage {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		writePlainTable(w, rows)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, "<!doctype html><meta charset=utf-8><title>CX benchmark runs</title>"+
		"<style>body{background:#272822;color:#f8f8f2;font:14px monospace}td,th{padding:2px 10px;text-align:left}</style>"+
		"<table><tr>")
	for _, h := range runTableHeader {
		fmt.Fprintf(w, "<th>%s</th>", html.EscapeString(h))
	}
	io.WriteString(w, "</tr>\n")
	for _, row := range rows {
		io.WriteString(w, "<tr>")
		for _, cell := range row {
			fmt.Fprintf(w, "<td>%s</td>", html.EscapeString(cell))
		}
		io.WriteString(w, "</tr>\n")
	}
	io.WriteString(w, "</table>\n")
}

func writePlainTable(w io.Writer, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(runTableHeader, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	tw.Flush()
}

// cleanCells keeps submitted text from breaking the table layout.
func cleanCells(cells ...string) []string {
	for i, c := range cells {
		cells[i] = strings.Map(func(r rune) rune {
			if r < ' ' {
				return ' '
			}
			return r
		}, c)
	}
	return cells
}
