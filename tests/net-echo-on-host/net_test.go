// Package netechohost runs the network test programs on the host against
// netecho, with the emulator's host address 10.0.2.2:7001 replaced by the
// server's address.
package netechohost

import (
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skycoin/skycoin-cx-playground-android/cxrun"
	"github.com/skycoin/skycoin-cx-playground-android/netecho"
)

// source reads a test program with the emulator's host address replaced by
// addr, in the app.NetDial form and in the sys/net form of a host and a port.
func source(t *testing.T, path, addr string) map[string][]byte {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	src = bytes.ReplaceAll(src, []byte("10.0.2.2:7001"), []byte(addr))
	src = bytes.ReplaceAll(src, []byte(`"10.0.2.2", 7001`), []byte(`"`+host+`", `+port))
	return map[string][]byte{"main.cx": src}
}

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
		var out bytes.Buffer
		if err := cxrun.Run(source(t, c.app, s.Addr), "main.cx", nil, &out); err != nil {
			t.Fatalf("%s: %v\n%s", c.app, err, out.String())
		}
		if !strings.Contains(out.String(), c.want) {
			t.Errorf("%s printed %q, want %q in it", c.app, out.String(), c.want)
		}
	}
}

// TestNetReplay records a live run of app.Net* and of sys/net programs,
// stops the server, and replays the record: the replay opens no socket and
// prints what the live run printed (skycoin-cx#508).
func TestNetReplay(t *testing.T) {
	for _, c := range []struct {
		program   string
		libraries []string
		want      string
	}{
		{"../net-tcp-echo/app/main.cx", nil, "tcp echoed 5 of 5\n"},
		{"programs/sysnet-tcp-echo.cx", []string{"net"}, "echoed 30 bytes\n"},
	} {
		t.Run(filepath.Base(filepath.Dir(c.program))+"/"+filepath.Base(c.program), func(t *testing.T) {
			s, err := netecho.Listen("127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			files := source(t, c.program, s.Addr)
			record := filepath.Join(t.TempDir(), "run.netreplay")
			var live bytes.Buffer
			err = cxrun.RunWith(files, "main.cx", cxrun.Options{Libraries: c.libraries, NetRecord: record}, nil, &live)
			s.Close()
			if err != nil {
				t.Fatalf("live: %v\n%s", err, live.String())
			}
			if !strings.Contains(live.String(), c.want) {
				t.Fatalf("live run printed %q, want %q in it", live.String(), c.want)
			}
			for range 2 {
				var replay bytes.Buffer
				if err := cxrun.RunWith(files, "main.cx", cxrun.Options{Libraries: c.libraries, NetReplay: record}, nil, &replay); err != nil {
					t.Fatalf("replay: %v\n%s", err, replay.String())
				}
				if replay.String() != live.String() {
					t.Fatalf("replay printed %q, live run %q", replay.String(), live.String())
				}
			}
		})
	}
}

// TestNetRecvTimeoutReplays checks that the timeout of app.NetRecv is an
// entry of a batch: a peer that never answers gives the same timeouts live
// and in the replay, and an EOF from a peer that closed repeats.
func TestNetRecvTimeoutReplays(t *testing.T) {
	silent, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer silent.Close()
	closing, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer closing.Close()
	go func() {
		if c, err := closing.Accept(); err == nil {
			c.Write([]byte("bye"))
			c.Close()
		}
	}()
	src := `package main

import "sys/app"

func main() {
	var conn i32
	var err str
	var data str
	conn, err = app.NetDial("udp", "UDP")
	err = app.NetSend(conn, "anyone")
	for i := 0; i < 3; i++ {
		data, err = app.NetRecv(conn, 30)
		printf("udp %d [%s] %s\n", i, data, err)
	}
	conn, err = app.NetDial("tcp", "TCP")
	for i := 0; i < 3; i++ {
		data, err = app.NetRecv(conn, 1000)
		printf("tcp %d [%s] %s\n", i, data, err)
	}
}
`
	src = strings.Replace(src, "UDP", silent.LocalAddr().String(), 1)
	src = strings.Replace(src, "TCP", closing.Addr().String(), 1)
	want := "udp 0 [] timeout\nudp 1 [] timeout\nudp 2 [] timeout\ntcp 0 [bye] \ntcp 1 [] EOF\ntcp 2 [] EOF\n"
	files := map[string][]byte{"main.cx": []byte(src)}
	record := filepath.Join(t.TempDir(), "run.netreplay")
	for _, opts := range []cxrun.Options{{NetRecord: record}, {NetReplay: record}} {
		var out bytes.Buffer
		if err := cxrun.RunWith(files, "main.cx", opts, nil, &out); err != nil {
			t.Fatalf("%+v: %v\n%s", opts, err, out.String())
		}
		if out.String() != want {
			t.Fatalf("%+v printed %q, want %q", opts, out.String(), want)
		}
	}
}
