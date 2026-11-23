//go:build cxopcount

// Package benchmarkopcounts checks the operation counts in the benchmark
// app's suite.json, which the app divides by its median times for ops/ms.
// Run it with: go test -tags cxopcount ./gotests/benchmark-opcounts
package benchmarkopcounts

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/benchsuite"
	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
	vm "github.com/skycoin/skycoin-cx/t3-cx-runtime-vm"
)

func TestSuiteOperationCounts(t *testing.T) {
	dir := "../../apps/benchmarks"
	b, err := os.ReadFile(filepath.Join(dir, benchsuite.SuiteFile))
	if err != nil {
		t.Fatal(err)
	}
	suite, err := benchsuite.ParseSuiteDefinition(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, bench := range suite.Benchmarks {
		src, err := os.ReadFile(filepath.Join(dir, bench.File))
		if err != nil {
			t.Fatal(err)
		}
		for _, executor := range benchsuite.Executors {
			counter := &cxast.OperationCount
			if executor == cxrun.ExecutorVm {
				counter = &vm.OperationCount
			}
			before := *counter
			if _, err := cxrun.RunOn(map[string][]byte{"main.cx": src}, "main.cx", executor, nil, io.Discard); err != nil {
				t.Fatal(bench.Name, executor, err)
			}
			if got := *counter - before; got != bench.Operations[string(executor)] {
				t.Errorf("%s %s: counted %d operations, suite.json has %d", bench.Name, executor, got, bench.Operations[string(executor)])
			}
		}
	}
}
