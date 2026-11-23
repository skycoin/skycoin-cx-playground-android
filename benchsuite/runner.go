package benchsuite

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
)

// AppVersion is the version of the playground reported in results.
const AppVersion = "0.2.0"

// Executors are the executors each benchmark runs on, in order.
var Executors = []cxrun.Executor{cxrun.ExecutorOld, cxrun.ExecutorVm}

// RunSuite runs every benchmark of the suite in files on every executor,
// runs times each, and writes one progress line per run to progress. runs
// below 1 takes the suite's own count.
func RunSuite(files map[string][]byte, runs int, progress io.Writer) (*ResultsDocument, error) {
	suiteBytes, found := files[SuiteFile]
	if !found {
		return nil, fmt.Errorf("no %s", SuiteFile)
	}
	suite, err := ParseSuiteDefinition(suiteBytes)
	if err != nil {
		return nil, err
	}
	if runs < 1 {
		runs = suite.Runs
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	doc := &ResultsDocument{
		Schema:    SchemaName,
		RunID:     newRunID(),
		StartedAt: time.Now().UTC().Truncate(time.Millisecond),
		Build:     CollectAppBuild(),
		Device:    CollectDeviceProfile(),
		Suite:     SuiteSettings{Scale: suite.Scale, Sizes: suite.Sizes, Runs: runs},
	}
	fmt.Fprintf(progress, "suite %s scale, %d runs per benchmark and executor\n", suite.Scale, runs)
	for _, bench := range suite.Benchmarks {
		source, found := files[bench.File]
		if !found {
			return nil, fmt.Errorf("benchmark %s: no file %s", bench.Name, bench.File)
		}
		for _, executor := range Executors {
			result := RunBenchmark(bench, source, executor, runs)
			fmt.Fprintln(progress, ResultLine(result))
			doc.Results = append(doc.Results, result)
		}
	}
	doc.FinishedAt = time.Now().UTC().Truncate(time.Millisecond)
	return doc, nil
}

// RunBenchmark runs one program runs times on executor.
func RunBenchmark(bench SuiteBenchmark, source []byte, executor cxrun.Executor, runs int) BenchmarkResult {
	result := BenchmarkResult{
		Benchmark:    bench.Name,
		Executor:     string(executor),
		ExpectedHash: bench.ExpectedHash,
		RunsMs:       []float64{},
	}
	files := map[string][]byte{"main.cx": source}
	status := StatusPass
	for run := 0; run < runs; run++ {
		var out bytes.Buffer
		elapsed, err := cxrun.RunOn(files, "main.cx", executor, nil, &out)
		if err != nil {
			result.Status, result.Message = StatusError, strings.TrimSpace(err.Error()+"\n"+lastLines(out.String(), 4))
			if errors.Is(err, cxrun.ErrVmRejected) {
				result.Status, result.Message = StatusRejected, err.Error()
			}
			result.RunsMs = []float64{}
			return result
		}
		hash, passLine := parseBenchmarkOutput(out.String(), bench.Name)
		result.Hash = hash
		if hash != bench.ExpectedHash || !passLine {
			status = StatusFail
			result.Message = "output: " + lastLines(out.String(), 4)
		}
		result.RunsMs = append(result.RunsMs, float64(elapsed.Microseconds())/1000)
	}
	result.Status = status
	result.MedianMs = median(result.RunsMs)
	if ops := bench.Operations[string(executor)]; ops > 0 && result.MedianMs > 0 {
		result.Operations = ops
		result.OpsPerMs = float64(int64(float64(ops)/result.MedianMs*10)) / 10
	}
	return result
}

// ResultLine is a one line summary of a result for the console.
func ResultLine(r BenchmarkResult) string {
	switch r.Status {
	case StatusPass, StatusFail:
		line := fmt.Sprintf("%-8s %-3s %-4s %10.1f ms", r.Benchmark, r.Executor, r.Status, r.MedianMs)
		if r.OpsPerMs > 0 {
			line += fmt.Sprintf(" %9.0f ops/ms", r.OpsPerMs)
		}
		return line
	}
	return fmt.Sprintf("%-8s %-3s %s: %s", r.Benchmark, r.Executor, r.Status, r.Message)
}

// parseBenchmarkOutput returns the hash line and whether "<name> pass" was
// printed.
func parseBenchmarkOutput(out, name string) (hash string, passLine bool) {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == name+" pass":
			passLine = true
		case hash == "" && line != "" && strings.Trim(line, "0123456789") == "":
			hash = line
		}
	}
	return hash, passLine
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return strings.Join(lines[max(len(lines)-n, 0):], "\n")
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}

func newRunID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// CollectAppBuild reads the versions Go stamped into the binary.
func CollectAppBuild() AppBuild {
	build := AppBuild{AppVersion: AppVersion, GoVersion: runtime.Version(), SkycoinCx: "unknown"}
	info, found := debug.ReadBuildInfo()
	if !found {
		return build
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/skycoin/skycoin-cx" {
			build.SkycoinCx = dep.Version
		}
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			build.AppRevision = setting.Value
		}
	}
	return build
}
