// Package netechohost runs the network test programs on the host against
// netecho, with the emulator's host address 10.0.2.2:7001 replaced by the
// server's address.
package netechohost

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/netecho"
)

func TestNetPrograms(t *testing.T) {
	s, err := netecho.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, c := range []struct{ app, want string }{
		{"../net-tcp-echo/app/main.cx", "tcp echoed 5 of 5\n"},
		{"../net-udp-echo/app/main.cx", "udp received 20 of 20\n"},
		{"../net-udp-echo/app/main.cx", "no loss\n"},
	} {
		src, err := os.ReadFile(c.app)
		if err != nil {
			t.Fatal(err)
		}
		src = bytes.ReplaceAll(src, []byte("10.0.2.2:7001"), []byte(s.Addr))
		var out bytes.Buffer
		if err := cxrun.Run(map[string][]byte{"main.cx": src}, "main.cx", nil, &out); err != nil {
			t.Fatalf("%s: %v\n%s", c.app, err, out.String())
		}
		if !strings.Contains(out.String(), c.want) {
			t.Errorf("%s printed %q, want %q in it", c.app, out.String(), c.want)
		}
	}
}
