// Package benchsuite runs the skycoin-cx t0-benchmarks/compare programs on
// both executors, collects a device profile and builds the results document
// that the app shows, saves and uploads.
package benchsuite

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"

	"github.com/skycoin/skycoin-cx-playground-android/textform"
)

// SuiteFile is the file of the benchmark app that lists its programs.
const SuiteFile = "suite.txt"

// BenchmarkNames are the compare programs in the order they run.
var BenchmarkNames = []string{"count", "fib", "array", "struct", "memtest", "primes"}

// Sizes are the parameters of one scale of the compare programs, the fields
// of compareScale in skycoin-cx t0-benchmarks/compare_test.go.
type Sizes struct {
	CountN       uint64
	FibN         int
	ArrayRounds  int
	StructRounds int
	MemN         uint64
	MemWalk      uint64
	SieveN       int64
	MillerN      int64
	LucasRounds  int
}

// SuiteBenchmark is one program of the suite.
type SuiteBenchmark struct {
	Name string
	File string
	// ExpectedHash is the FNV-1a hash the program prints, in decimal.
	ExpectedHash string
	// Operations are the operations each executor runs, counted on a host
	// with the cxopcount build tag, so the app reports ops/ms without the
	// cost of counting.
	Operations map[string]uint64
}

// SuiteDefinition is the content of SuiteFile.
type SuiteDefinition struct {
	Scale      string
	Sizes      Sizes
	Runs       int
	Benchmarks []SuiteBenchmark
}

// ParseSuiteDefinition reads and checks a SuiteFile in the text form: a
// [suite] section with scale and runs, a [sizes] section with the fields
// of Sizes, and a [benchmarks] table of name, file, expected_hash and the
// operations of the old and the vm executor.
func ParseSuiteDefinition(b []byte) (*SuiteDefinition, error) {
	suite, err := parseSuite(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SuiteFile, err)
	}
	if suite.Runs < 1 {
		return nil, errors.New("suite runs must be at least 1")
	}
	if len(suite.Benchmarks) == 0 {
		return nil, errors.New("suite lists no benchmarks")
	}
	for _, b := range suite.Benchmarks {
		if _, err := strconv.ParseUint(b.ExpectedHash, 10, 64); err != nil {
			return nil, fmt.Errorf("benchmark %s: expected_hash: %w", b.Name, err)
		}
	}
	return suite, nil
}

// sizeFields maps the [sizes] keys to the fields of Sizes.
func sizeFields(s *Sizes) map[string]any {
	return map[string]any{
		"count_n": &s.CountN, "fib_n": &s.FibN, "array_rounds": &s.ArrayRounds, "struct_rounds": &s.StructRounds,
		"mem_n": &s.MemN, "mem_walk": &s.MemWalk, "sieve_n": &s.SieveN, "miller_n": &s.MillerN, "lucas_rounds": &s.LucasRounds,
	}
}

// setNumber parses text into the integer field p points to.
func setNumber(p any, text string) error {
	switch v := p.(type) {
	case *int:
		n, err := strconv.Atoi(text)
		*v = n
		return err
	case *int64:
		n, err := strconv.ParseInt(text, 10, 64)
		*v = n
		return err
	case *uint64:
		n, err := strconv.ParseUint(text, 10, 64)
		*v = n
		return err
	}
	return fmt.Errorf("field of type %T", p)
}

func parseSuite(b []byte) (*SuiteDefinition, error) {
	d, err := textform.Parse(string(b))
	if err != nil {
		return nil, err
	}
	if err := d.Only("suite", "sizes", "benchmarks"); err != nil {
		return nil, err
	}
	suite := &SuiteDefinition{}
	s, _ := d.Section("suite")
	k, err := s.Keys("scale", "runs")
	if err != nil {
		return nil, err
	}
	suite.Scale = k["scale"]
	if err := setNumber(&suite.Runs, k["runs"]); err != nil {
		return nil, fmt.Errorf("runs: %w", err)
	}
	fields := sizeFields(&suite.Sizes)
	names := make([]string, 0, len(fields))
	for n := range fields {
		names = append(names, n)
	}
	s, _ = d.Section("sizes")
	k, err = s.Keys(names...)
	if err != nil {
		return nil, err
	}
	for n, v := range k {
		if err := setNumber(fields[n], v); err != nil {
			return nil, fmt.Errorf("sizes %s: %w", n, err)
		}
	}
	s, _ = d.Section("benchmarks")
	rows, err := s.Rows(5)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		bench := SuiteBenchmark{Name: r[0], File: r[1], ExpectedHash: r[2], Operations: map[string]uint64{}}
		for i, executor := range []string{"old", "vm"} {
			n, err := strconv.ParseUint(r[3+i], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("benchmark %s operations %s: %w", r[0], executor, err)
			}
			bench.Operations[executor] = n
		}
		suite.Benchmarks = append(suite.Benchmarks, bench)
	}
	return suite, nil
}

const (
	fnvOffset = 14695981039346656037
	fnvPrime  = 1099511628211
)

func fnvFold(hash, value uint64) uint64 {
	return (hash ^ value) * fnvPrime
}

// primeCount counts the primes below n, as comparePrimeCount does.
func primeCount(n int64) uint64 {
	var count uint64
	if n < 1<<20 {
		for k := int64(2); k < n; k++ {
			if big.NewInt(k).ProbablyPrime(0) {
				count++
			}
		}
		return count
	}
	composite := make([]bool, n)
	for k := int64(2); k < n; k++ {
		if composite[k] {
			continue
		}
		count++
		for multiple := k * k; multiple < n; multiple += k {
			composite[multiple] = true
		}
	}
	return count
}

// ExpectedHash derives the hash a compare program prints at sizes from the
// math, a copy of compareExpected in skycoin-cx t0-benchmarks/compare_test.go.
func ExpectedHash(name string, sizes Sizes) (uint64, error) {
	hash := uint64(fnvOffset)
	switch name {
	case "count":
		hash = fnvFold(hash, sizes.CountN*(sizes.CountN-1)/2)
	case "fib":
		previous, current := uint64(0), uint64(1)
		for i := 0; i < sizes.FibN; i++ {
			previous, current = current, previous+current
		}
		hash = fnvFold(hash, previous)
	case "array":
		for round := 0; round < sizes.ArrayRounds; round++ {
			hash = fnvFold(hash, 150*149/2)
		}
		for value := uint64(1); value <= 150; value++ {
			hash = fnvFold(hash, value)
		}
	case "struct":
		for round := 0; round < sizes.StructRounds; round++ {
			hash = fnvFold(hash, 3*99*100/2)
		}
	case "memtest":
		const all = 0xFFFFFFFF
		hash = fnvFold(fnvFold(hash, 0), sizes.MemN*all)
		for bit := 0; bit < 32; bit++ {
			hash = fnvFold(fnvFold(hash, 0), sizes.MemWalk<<bit)
		}
		for _, pattern := range []uint64{0, all, 0x55555555, 0xAAAAAAAA} {
			hash = fnvFold(fnvFold(hash, 0), sizes.MemN*(pattern+all))
		}
	case "primes":
		hash = fnvFold(hash, primeCount(sizes.SieveN))
		hash = fnvFold(hash, primeCount(sizes.MillerN))
		// The Mersenne prime exponents up to 61, OEIS A000043.
		mersenne := map[uint64]bool{3: true, 5: true, 7: true, 13: true, 17: true, 19: true, 31: true, 61: true}
		for round := 0; round < sizes.LucasRounds; round++ {
			for _, exponent := range []uint64{3, 5, 7, 11, 13, 17, 19, 23, 29, 31, 37, 41, 43, 47, 53, 59, 61} {
				flag := uint64(0)
				if mersenne[exponent] {
					flag = 1
				}
				hash = fnvFold(hash, flag)
			}
		}
	default:
		return 0, fmt.Errorf("unknown benchmark %q", name)
	}
	return hash, nil
}
