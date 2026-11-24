// Test program-validation-network-udp checks that the UDP echo app reports
// UDP, the TCP echo app TCP, and a network name that is not a constant
// unknown.
package networkudp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/programcheck"
)

func check(t *testing.T, path string) *programcheck.Report {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r, err := programcheck.CheckSource(map[string][]byte{"main.cx": src}, "main.cx", programcheck.PolicyFetched())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestNetworkUse(t *testing.T) {
	for _, c := range []struct {
		path string
		line string
	}{
		{filepath.Join("..", "net-udp-echo", "app", "main.cx"), "networking yes, udp yes, tcp no"},
		{filepath.Join("..", "net-tcp-echo", "app", "main.cx"), "networking yes, udp no, tcp yes"},
		{filepath.Join("programs", "network-from-args.cx"), "networking yes, udp unknown, tcp unknown"},
		{filepath.Join("..", "cx-source-fib15", "app", "main.cx"), "networking no"},
	} {
		r := check(t, c.path)
		if !strings.Contains(r.Text(), "\n"+c.line+"\n") {
			t.Errorf("%s: report has no line %q:\n%s", c.path, c.line, r.Text())
		}
	}
	udp := check(t, filepath.Join("..", "net-udp-echo", "app", "main.cx"))
	if !udp.UDP || udp.TCP || udp.NetworkUnknown || udp.NetworkCalls != 1 || !udp.Allowed() {
		t.Fatalf("udp echo: %+v", udp)
	}
}
