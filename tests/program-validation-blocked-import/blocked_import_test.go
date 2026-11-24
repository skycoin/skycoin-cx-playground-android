// Test program-validation-blocked-import checks that a fetched program that
// imports a blocked package is refused, even when it calls no system
// library, and that the same program runs as a bundled app.
package blockedimport

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programcheck"
)

func TestBlockedImportRefused(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("programs", "exits.cx"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"main.cx": src}
	r, err := programcheck.CheckSource(files, "main.cx", programcheck.PolicyFetched())
	if err != nil {
		t.Fatal(err)
	}
	if r.Allowed() || !slices.Equal(r.BlockedImports, []string{"os"}) || len(r.LibraryCalls) != 0 {
		t.Fatalf("blocked %v, library calls %v, violations %v", r.BlockedImports, r.LibraryCalls, r.Violations)
	}
	for _, line := range []string{"imports os", "blocked imports os", "sandbox violations 1", "  import os is blocked", "verdict refused"} {
		if !strings.Contains(r.Text(), "\n"+line+"\n") {
			t.Errorf("report has no line %q:\n%s", line, r.Text())
		}
	}
	bundled, err := programcheck.CheckSource(files, "main.cx", programcheck.PolicyBundled())
	if err != nil || !bundled.Allowed() {
		t.Fatalf("bundled: %v %v", err, bundled.Violations)
	}
}
