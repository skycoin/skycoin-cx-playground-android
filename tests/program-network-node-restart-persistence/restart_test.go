// Package programnetworknoderestartpersistence restarts a medium and a
// super node and checks what survives: the programs each held, the nodes
// each subscribed to (subscriptions.txt) and the subscribers each had
// (subscribers.txt). A program published after the restart reaches the
// medium node through the super node without any new Subscribe call, and
// a pin added to the pin list file while the node runs takes effect.
package programnetworknoderestartpersistence

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

var publishDate = time.Unix(1700000000, 0)

func plannedAddress(t *testing.T, files map[string][]byte, secret cipher.SecKey) programblocks.Hash {
	m, _, err := programblocks.Build(files, programblocks.DefaultBlockSize, publishDate)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Sign(secret); err != nil {
		t.Fatal(err)
	}
	return m.Address()
}

func startNode(t *testing.T, name, dir, addr string, trusted cipher.PubKey, settings programnetwork.Settings) *programnetwork.Node {
	t.Helper()
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		t.Fatal(err)
	}
	st, err := programstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	n, err := programnetwork.NewTypedNode(pc, st, []cipher.PubKey{trusted}, settings)
	if err != nil {
		t.Fatal(err)
	}
	n.Logf = func(format string, args ...any) { t.Logf(name+": "+format, args...) }
	go n.Run()
	t.Cleanup(func() { n.Close() })
	return n
}

func waitFor(t *testing.T, what string, limit time.Duration, done func() bool) {
	t.Helper()
	start := time.Now()
	for !done() {
		if time.Since(start) > limit {
			t.Fatalf("%s: not after %v", what, limit)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func readFile(t *testing.T, path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRestartKeepsProgramsAndSubscriptions(t *testing.T) {
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("program network restart"))
	var progs []map[string][]byte
	var addrs []programblocks.Hash
	for _, app := range []string{"bench-fib", "triangles", "bench-loops"} {
		files, err := programblocks.ReadProgramDirectory("../../apps/" + app)
		if err != nil {
			t.Fatal(err)
		}
		progs, addrs = append(progs, files), append(addrs, plannedAddress(t, files, secret))
	}
	superDir, mediumDir := t.TempDir(), t.TempDir()
	pinsFile := filepath.Join(t.TempDir(), "pins.txt")
	if err := programnetwork.WritePins(pinsFile, []string{addrs[0].String()}); err != nil {
		t.Fatal(err)
	}
	superSettings := programnetwork.Settings{Type: programnetwork.TypeSuper}
	mediumSettings := programnetwork.Settings{Type: programnetwork.TypeMedium, PinsFile: pinsFile}

	publisher := startNode(t, "publisher", t.TempDir(), "127.0.0.1:0", pub, superSettings)
	super := startNode(t, "super", superDir, "127.0.0.1:0", pub, superSettings)
	medium := startNode(t, "medium", mediumDir, "127.0.0.1:0", pub, mediumSettings)
	superAddr, mediumAddr := super.Addr().String(), medium.Addr().String()
	if err := super.Subscribe(publisher.Addr().String()); err != nil {
		t.Fatal(err)
	}
	if err := medium.Subscribe(superAddr); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "subscriptions", 5*time.Second, func() bool {
		return publisher.Stats().Subscribers == 1 && super.Stats().Subscribers == 1
	})
	for _, files := range progs[:2] {
		if _, err := publisher.Publish(files, secret, publishDate); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "first flood", 5*time.Second, func() bool {
		return super.Holds(addrs[0]) && super.Holds(addrs[1]) && medium.Holds(addrs[0])
	})
	superBytes, mediumBytes := super.Stats().StoredBytes, medium.Stats().StoredBytes
	super.Close()
	medium.Close()

	subscriptions := readFile(t, filepath.Join(superDir, programnetwork.SubscriptionsFile))
	subscribers := readFile(t, filepath.Join(superDir, programnetwork.SubscribersFile))
	if subscriptions != publisher.Addr().String()+"\n" || !strings.HasPrefix(subscribers, mediumAddr+" ") {
		t.Fatalf("super files: %s %q, %s %q", programnetwork.SubscriptionsFile, subscriptions, programnetwork.SubscribersFile, subscribers)
	}
	if got := readFile(t, filepath.Join(mediumDir, programnetwork.SubscriptionsFile)); got != superAddr+"\n" {
		t.Fatalf("medium %s %q", programnetwork.SubscriptionsFile, got)
	}
	t.Logf("super %s: %q, %s: %q", programnetwork.SubscriptionsFile, subscriptions, programnetwork.SubscribersFile, subscribers)

	// Restart both on their ports, with no Subscribe call. The super node
	// knows its subscriber from subscribers.txt before the medium node is
	// back.
	super = startNode(t, "super restarted", superDir, superAddr, pub, superSettings)
	if s := super.Stats(); !super.Holds(addrs[0]) || !super.Holds(addrs[1]) || s.StoredBytes != superBytes || s.Subscribers != 1 {
		t.Fatalf("restarted super: %+v, want both programs, %d bytes and its subscriber", s, superBytes)
	}
	medium = startNode(t, "medium restarted", mediumDir, mediumAddr, pub, mediumSettings)
	if s := medium.Stats(); !medium.Holds(addrs[0]) || medium.Holds(addrs[1]) || s.StoredBytes != mediumBytes {
		t.Fatalf("restarted medium: %+v, want only the pinned program, %d bytes", s, mediumBytes)
	}
	// A pin added while the medium node runs, as `programnetwork pins add`
	// does, applies to the next have.
	if err := programnetwork.WritePins(pinsFile, []string{addrs[0].String(), addrs[2].String()[:8]}); err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Publish(progs[2], secret, publishDate); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "flood after the restart", 5*time.Second, func() bool { return super.Holds(addrs[2]) && medium.Holds(addrs[2]) })
	if s := medium.Stats(); medium.Holds(addrs[1]) || s.Downloads != 1 {
		t.Errorf("restarted medium: %+v, want one new download, the newly pinned program", s)
	}
	t.Logf("after the restart: super holds 3 programs, %d bytes; medium holds the 2 pinned, %d bytes", super.Stats().StoredBytes, medium.Stats().StoredBytes)
}
