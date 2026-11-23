// Command benchmark-collector receives results documents from the benchmark
// suite app, stores them in a bbolt file and serves a table of runs.
//
//	benchmark-collector -addr 127.0.0.1:8090 -db runs.db
//
// POST /runs takes one document, GET /runs.txt and GET / list the runs.
package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/benchcollector"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address")
	dbPath := flag.String("db", "benchmark-runs.db", "bbolt file holding the runs")
	flag.Parse()
	runs, err := benchcollector.OpenRunStore(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer runs.Close()
	server := &http.Server{
		Addr:              *addr,
		Handler:           &benchcollector.CollectorServer{Runs: runs},
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
	log.Printf("benchmark-collector: listening on %s, storing in %s", *addr, *dbPath)
	log.Fatal(server.ListenAndServe())
}
