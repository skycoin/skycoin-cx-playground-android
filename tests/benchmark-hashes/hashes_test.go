// Package benchmarkhashes checks the expected hashes of the benchmark app
// against the math and runs every benchmark through the playground runner.
package benchmarkhashes

import (
	"os"
	"strconv"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/benchsuite"
	"github.com/skycoin/skycoin-cx-playground-android/store"
)

// skycoinCxExpected is skycoin-cx t0-benchmarks/compare/expected.txt at
// df4c987, for checking the copy of compareExpected in benchsuite.
var skycoinCxExpected = map[string]map[string]uint64{
	"full": {"count": 17370372864278721183, "fib": 173487872732728998, "array": 3441325112497614442, "struct": 2818881072322839333, "memtest": 874058405928975501, "primes": 13592104564658177911},
	"bash": {"count": 11410959382411819791, "fib": 12656982252328280830, "array": 8001745308867579662, "struct": 8983513230273059621, "memtest": 7377915752544696461, "primes": 4234126189016467776},
}

var skycoinCxSizes = map[string]benchsuite.Sizes{
	"full": {CountN: 30000000, FibN: 34, ArrayRounds: 400, StructRounds: 80000, MemN: 1048576, MemWalk: 65536, SieveN: 2000000, MillerN: 300000, LucasRounds: 20},
	"bash": {CountN: 300000, FibN: 22, ArrayRounds: 4, StructRounds: 800, MemN: 16384, MemWalk: 1024, SieveN: 200000, MillerN: 30000, LucasRounds: 1},
}

func TestExpectedHashMatchesSkycoinCx(t *testing.T) {
	for scale, hashes := range skycoinCxExpected {
		for name, want := range hashes {
			got, err := benchsuite.ExpectedHash(name, skycoinCxSizes[scale])
			if err != nil || got != want {
				t.Errorf("%s %s: got %d, %v, skycoin-cx has %d", name, scale, got, err, want)
			}
		}
	}
}

// bundledSuiteFiles loads the benchmark app from the embedded bundle, verified
// the way the app verifies it.
func bundledSuiteFiles(t *testing.T) map[string][]byte {
	src := store.FSSource{FS: os.DirFS("../../cmd/cxplayground/bundle")}
	apps, errs, err := store.List(src)
	if err != nil || len(errs) > 0 {
		t.Fatal(err, errs)
	}
	for _, app := range apps {
		if app.Manifest.Kind == "benchmark" {
			files, err := store.Load(src, app)
			if err != nil {
				t.Fatal(err)
			}
			return files
		}
	}
	t.Fatal("no benchmark app in the bundle")
	return nil
}

func TestSuiteHashesFromMath(t *testing.T) {
	suite, err := benchsuite.ParseSuiteDefinition(bundledSuiteFiles(t)[benchsuite.SuiteFile])
	if err != nil {
		t.Fatal(err)
	}
	if len(suite.Benchmarks) != len(benchsuite.BenchmarkNames) {
		t.Fatalf("%d benchmarks, want %d", len(suite.Benchmarks), len(benchsuite.BenchmarkNames))
	}
	for i, bench := range suite.Benchmarks {
		if bench.Name != benchsuite.BenchmarkNames[i] {
			t.Errorf("benchmark %d is %s, want %s", i, bench.Name, benchsuite.BenchmarkNames[i])
		}
		want, err := benchsuite.ExpectedHash(bench.Name, suite.Sizes)
		if err != nil || bench.ExpectedHash != strconv.FormatUint(want, 10) {
			t.Errorf("%s: suite has %s, the math gives %d", bench.Name, bench.ExpectedHash, want)
		}
	}
}

func TestSuiteRunsAndPasses(t *testing.T) {
	doc, err := benchsuite.RunSuite(bundledSuiteFiles(t), 1, testLogWriter{t})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Results) != 2*len(benchsuite.BenchmarkNames) {
		t.Fatalf("%d results", len(doc.Results))
	}
	for _, r := range doc.Results {
		if r.Status != benchsuite.StatusPass {
			t.Errorf("%s on %s: %s %s", r.Benchmark, r.Executor, r.Status, r.Message)
		}
	}
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	b, _ := benchsuite.EncodeResultsDocument(doc)
	t.Logf("results document:\n%s", b)
}

type testLogWriter struct{ t *testing.T }

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
