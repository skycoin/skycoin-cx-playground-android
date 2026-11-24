// Test program-validation-report-text compares the report text of two
// programs, checked as fetched programs, with the expected lines: the UDP
// echo app, which runs, and a program that reads a file, which is refused.
package reporttext

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programcheck"
)

var update = flag.Bool("update", false, "write the expected reports")

func TestReportText(t *testing.T) {
	for _, c := range []struct{ program, expected string }{
		{filepath.Join("..", "net-udp-echo", "app", "main.cx"), "udp-echo.txt"},
		{filepath.Join("..", "program-validation-library-calls", "programs", "file.cx"), "reads-a-file.txt"},
	} {
		src, err := os.ReadFile(c.program)
		if err != nil {
			t.Fatal(err)
		}
		r, err := programcheck.CheckSource(map[string][]byte{"main.cx": src}, "main.cx", programcheck.PolicyFetched())
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("expected", c.expected)
		if *update {
			if err := os.WriteFile(path, []byte(r.Text()), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if r.Text() != string(want) {
			t.Errorf("%s report:\n%s\nwant:\n%s", c.program, r.Text(), want)
		}
	}
}
