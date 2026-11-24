// Package programnetworkpeerlistfileroundtrip builds a peer list with the
// programnetwork peers command, one change per run, and checks the file
// it writes: it parses back to the same list, formats to the same bytes,
// and every command shows up in it.
package programnetworkpeerlistfileroundtrip

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	run("group-add", "lan1", "lan", "moe", "moe", "home")
	run("group-add", "china1", "internal", "lab", "isp-a", "china")
	run("group-add", "global", "global", "-", "-", "global")
	run("add", "192.168.1.5:7010", "lan1", "lan-group", "laptop", "in", "the", "office")
	run("add", "10.1.0.1:7010", "china1", "own-infrastructure")
	run("add", "203.0.113.5:7010", "global", "user-shareable")
	run("add", "203.0.113.6:7010", "global", "private")
	run("set-class", "203.0.113.5:7010", "public")
	run("set-group", "10.1.0.1:7010", "global")
	run("remove", "203.0.113.6:7010")
	run("group-remove", "china1")
	if out, err := exec.Command(bin, "peers", "-file", file, "group-remove", "lan1").CombinedOutput(); err == nil {
		t.Fatalf("a group with a peer was removed: %s", out)
	}
	if out, err := exec.Command(bin, "peers", "-file", file, "add", "10.9.9.9:1", "nogroup", "public").CombinedOutput(); err == nil {
		t.Fatalf("a peer of an unknown group was added: %s", out)
	}

	want := `# group NAME SCOPE OWNER OPERATOR SECURITY-GROUP
group global global - - global
group lan1 lan moe moe home
# peer ADDRESS GROUP CLASS [COMMENT]
peer 10.1.0.1:7010 global own-infrastructure
peer 192.168.1.5:7010 lan1 lan-group laptop in the office
peer 203.0.113.5:7010 global public
`
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != want {
		t.Fatalf("peers.txt:\n%s\nwant:\n%s", b, want)
	}
	if got := run("list"); got != want {
		t.Fatalf("list printed:\n%s", got)
	}
	l, err := programnetwork.ParsePeerList(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if string(l.Format()) != want {
		t.Fatalf("format after parse:\n%s", l.Format())
	}
	l2, err := programnetwork.ParsePeerList(string(l.Format()))
	if err != nil || !reflect.DeepEqual(l, l2) {
		t.Fatalf("second round trip differs: %v", err)
	}
	if got := run("shared-with", "203.0.113.200:7010"); got != "203.0.113.5:7010 global global\n" {
		t.Fatalf("shared with a stranger: %q", got)
	}
	t.Logf("peers.txt after 11 commands:\n%s", b)
}
