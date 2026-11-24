// Package programnetworkpeerlistfileroundtrip builds a peer list with the
// programnetwork peers command, one change per run, and checks the file
// it writes: the text form of #41 (a header line, one entry per line),
// which parses back to the same list and formats to the same bytes, with
// public peers shareable and private peers not.
package programnetworkpeerlistfileroundtrip

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
)

func TestPeerCommandRoundTrip(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "programnetwork")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/programnetwork")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	file := filepath.Join(dir, "peers.txt")
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(bin, append([]string{"peers", "-file", file}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("peers %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	run("add", "192.168.1.5:7010", "private")
	run("add", "10.1.0.1:7010", "private")
	run("add", "203.0.113.5:7010", "public")
	run("add", "[2001:db8::5]:7010", "public")
	run("add", "203.0.113.6:7010", "public")
	run("add", "10.1.0.1:7010", "public")
	run("remove", "203.0.113.6:7010")
	for _, bad := range [][]string{{"add", "node7.example.org:7010", "public"}, {"add", "203.0.113.7:7010", "shared"}, {"remove", "203.0.113.99:7010"}} {
		if out, err := exec.Command(bin, append([]string{"peers", "-file", file}, bad...)...).CombinedOutput(); err == nil {
			t.Fatalf("peers %s worked: %s", strings.Join(bad, " "), out)
		}
	}

	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	// Last seen is the time of each add, so it is matched as digits.
	want := `; CX peer list, text form. Flags bit 0 marks a peer shareable; private peers are never sent or learned.
[peers]
; address, last_seen, flags
10.1.0.1:7010, N, 1
192.168.1.5:7010, N, 0
203.0.113.5:7010, N, 1
[2001:db8::5]:7010, N, 1
`
	if got := regexp.MustCompile(`, \d+, `).ReplaceAllString(string(b), ", N, "); got != want {
		t.Fatalf("peers.txt:\n%s\nwant:\n%s", b, want)
	}
	if got := run("list"); got != string(b) {
		t.Fatalf("list printed:\n%s", got)
	}
	l, err := programnetwork.ParsePeerList(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if string(l.Format()) != string(b) {
		t.Fatalf("format after parse:\n%s", l.Format())
	}
	l2, err := programnetwork.ParsePeerList(string(l.Format()))
	if err != nil || !reflect.DeepEqual(l, l2) {
		t.Fatalf("second round trip differs: %v", err)
	}
	shareable := run("shareable")
	if strings.Contains(shareable, "192.168.1.5") || strings.Count(shareable, "\n") != 3 {
		t.Fatalf("shareable peers:\n%s", shareable)
	}
	for _, bad := range []string{"[peers]\nnode.example.org:1, 0, 1\n", "203.0.113.5:7010, 0, 1\n", "[groups]\n", "[peers]\n203.0.113.5:7010, 0, 2\n", "[peers]\n203.0.113.5:7010, 0\n"} {
		if _, err := programnetwork.ParsePeerList(bad); err == nil {
			t.Errorf("parsed %q", bad)
		}
	}
	t.Logf("peers.txt after 7 commands:\n%s", b)
}
