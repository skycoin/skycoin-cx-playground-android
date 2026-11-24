// Test program-validation-library-calls checks one program per system
// library: the check names the library and the function it calls, allows it
// for a bundled app and refuses it for a fetched program.
package librarycalls

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programcheck"
)

func check(t *testing.T, name string, p programcheck.Policy) *programcheck.Report {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("programs", name+".cx"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := programcheck.CheckSource(map[string][]byte{"main.cx": src}, "main.cx", p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOneProgramPerLibrary(t *testing.T) {
	for _, c := range []struct {
		program  string
		library  string
		function string
	}{
		{"file", "file", "os.ReadAllText"},
		{"net", "net", "net.TCPDial"},
		{"process", "process", "os.Run"},
	} {
		t.Run(c.program, func(t *testing.T) {
			r := check(t, c.program, programcheck.PolicyBundled())
			if !slices.Contains(r.LibraryCalls, programcheck.LibraryCall{Library: c.library, Function: c.function, Calls: 1}) {
				t.Fatalf("library calls %v, want %s %s", r.LibraryCalls, c.library, c.function)
			}
			for _, other := range []string{"file", "net", "process"} {
				if r.LibraryUsed(other) != (other == c.library) {
					t.Errorf("LibraryUsed(%s) = %v", other, r.LibraryUsed(other))
				}
			}
			if !r.Allowed() {
				t.Errorf("refused for a bundled app: %v", r.Violations)
			}
			if got := r.LibrariesToEnable(programcheck.PolicyBundled()); !slices.Equal(got, []string{c.library}) {
				t.Errorf("libraries to enable %v, want %s", got, c.library)
			}
			fetched := check(t, c.program, programcheck.PolicyFetched())
			if fetched.Allowed() || !slices.Contains(fetched.Violations, "library "+c.library+" is not allowed") {
				t.Errorf("fetched program violations %v, want library %s not allowed", fetched.Violations, c.library)
			}
		})
	}
	r := check(t, "none", programcheck.PolicyFetched())
	if len(r.LibraryCalls) != 0 || !r.Allowed() {
		t.Fatalf("a program without library calls: calls %v, violations %v", r.LibraryCalls, r.Violations)
	}
}
