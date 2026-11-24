// Test program-validation-opcode-sets checks the opcodes of a program
// against the sets a policy allows: an app opcode in a core-only program
// fails, as does a system library call where the library set is not
// allowed, and the serialized form gives the same result as the source.
package opcodesets

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/programcheck"
)

func read(t *testing.T, path string) map[string][]byte {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{"main.cx": src}
}

func TestOpcodeSets(t *testing.T) {
	coreOnly := programcheck.Policy{OpcodeSets: []programcheck.OpcodeSet{programcheck.OpcodeSetCore}}

	app := read(t, filepath.Join("..", "ui-tap", "app", "main.cx"))
	r, err := programcheck.CheckSource(app, "main.cx", coreOnly)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.OpcodeSets, []programcheck.OpcodeSet{programcheck.OpcodeSetCore, programcheck.OpcodeSetApp}) || r.Allowed() {
		t.Fatalf("sets %v, violations %v: an app opcode in a core-only program must fail", r.OpcodeSets, r.Violations)
	}

	core := read(t, filepath.Join("..", "cx-source-fib15", "app", "main.cx"))
	r, err = programcheck.CheckSource(core, "main.cx", coreOnly)
	if err != nil || !r.Allowed() || !slices.Equal(r.OpcodeSets, []programcheck.OpcodeSet{programcheck.OpcodeSetCore}) {
		t.Fatalf("core program: %v sets %v violations %v", err, r.OpcodeSets, r.Violations)
	}
	ir, err := cxrun.Serialize(core, "main.cx")
	if err != nil {
		t.Fatal(err)
	}
	fromIR, err := programcheck.CheckIR(ir, coreOnly)
	if err != nil || fromIR.Operations != r.Operations || fromIR.Functions != r.Functions || !fromIR.Allowed() {
		t.Fatalf("ir: %v, %d operations in %d functions, source %d in %d", err, fromIR.Operations, fromIR.Functions, r.Operations, r.Functions)
	}

	library := read(t, filepath.Join("..", "program-validation-library-calls", "programs", "file.cx"))
	noLibrarySet := programcheck.Policy{OpcodeSets: []programcheck.OpcodeSet{programcheck.OpcodeSetCore, programcheck.OpcodeSetApp}, Libraries: cxrun.LibrariesAll()}
	r, err = programcheck.CheckSource(library, "main.cx", noLibrarySet)
	if err != nil || r.Allowed() || !slices.Contains(r.Violations, "opcode set library is not allowed (1 operations)") {
		t.Fatalf("library call without the library set: %v %v", err, r.Violations)
	}
}
