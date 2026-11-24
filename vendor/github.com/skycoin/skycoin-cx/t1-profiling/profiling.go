package profiling

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/skycoin/skycoin-cx/t0-util" // thin: only CXCreateFile; no depth gain
)

// VIOLATION: state outside State. Owner: t1-profiling; moves to ConfigMng in phase 2.
var DebugProfile bool

// VIOLATION: state outside State. Owner: t1-profiling; moves to ConfigMng in phase 2.
var DebugProfileRate int

// VIOLATION: state outside State. Owner: t1-profiling; moves to ProfileMng in phase 1.
var profiles = make(map[string]int64)

// VIOLATION: state outside State. Owner: t1-profiling; moves to ProfileMng in phase 1.
// pprof runs one CPU profile at a time, this stops it.
var stopCPU func() error

func StartProfile(name string) {
	if DebugProfile {
		profiles[name] = time.Now().UnixNano()
	}
}

func StopProfile(name string) {
	if DebugProfile {
		t := time.Now().UnixNano()
		deltaTime := t - profiles[name]
		fmt.Printf("%s : %dms\n", name, deltaTime/(int64(time.Millisecond)))
	}
}

// StartCPUProfile writes a CPU profile to <argv0>_<name>_cpu.pprof at
// profRate samples per second. A rate of 0 does nothing. The returned
// function stops the profile and closes the file.
func StartCPUProfile(name string, profRate int) (stop func() error, err error) {
	return StartCPUProfileFile(fmt.Sprintf("%s_%s_cpu.pprof", os.Args[0], name), profRate)
}

// StartCPUProfileFile is StartCPUProfile writing to path.
func StartCPUProfileFile(path string, profRate int) (stop func() error, err error) {
	if profRate == 0 {
		return func() error { return nil }, nil
	}

	f, err := util.CXCreateFile(path)
	if err != nil {
		return nil, fmt.Errorf("create CPU profile: %w", err)
	}
	return StartCPUProfileOnFile(f, profRate)
}

// StartCPUProfileOnFile is StartCPUProfile writing to f at a nonzero profRate.
// It closes f on stop or failure.
// VIOLATION STRHOT-016: B, hands a path or text to Go or the OS, #412.
func StartCPUProfileOnFile(f *os.File, profRate int) (stop func() error, err error) {
	// test against default value to avoid warning
	if profRate != 100 {
		runtime.SetCPUProfileRate(profRate)
	}

	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("start CPU profile: %w", err)
	}

	stopped := false
	stop = func() error {
		if stopped {
			return nil
		}
		stopped = true
		stopCPU = nil
		pprof.StopCPUProfile()
		return f.Close()
	}
	stopCPU = stop
	return stop, nil
}

// DumpMemProfile writes a heap profile to <argv0>_<name>_mem.pprof.
func DumpMemProfile(name string) (err error) {
	f, err := util.CXCreateFile(fmt.Sprintf("%s_%s_mem.pprof", os.Args[0], name))
	if err != nil {
		return fmt.Errorf("create MEM profile: %w", err)
	}

	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()

	runtime.GC()
	if err = pprof.WriteHeapProfile(f); err != nil {
		return fmt.Errorf("write MEM profile: %w", err)
	}

	return nil
}

// CleanupAndExit stops a running CPU profile and exits with exitCode.
func CleanupAndExit(exitCode int) {
	if stopCPU != nil {
		if err := stopCPU(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	os.Exit(exitCode)
}
