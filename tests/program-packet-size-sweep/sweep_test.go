// Package programpacketsizesweep runs every program operation at packet
// sizes from 1400 down to 16 bytes, over UDP on the host, and writes the
// table of docs/ProgramDistribution.md "Packet size sweep": for each
// operation and size, the packets and bytes it takes for the reference
// program apps/benchmarks, or that it fails. Each operation's floor, the
// smallest size its packet layout allows, is checked against the runs.
//
// go test ./tests/program-packet-size-sweep -update rewrites sweep-table.md.
package programpacketsizesweep

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/cipher"

	"github.com/skycoin/skycoin-cx-playground-android/programblocks"
	"github.com/skycoin/skycoin-cx-playground-android/programnetwork"
	"github.com/skycoin/skycoin-cx-playground-android/programstore"
)

var update = flag.Bool("update", false, "rewrite sweep-table.md")

var sweepSizes = []int{1400, 1200, 1024, 512, 256, 128, 64, 32, 16}

const tablePath = "sweep-table.md"

// tally counts each distinct packet once, so a retransmission on a slow
// machine does not change the table.
type tally struct {
	mu      sync.Mutex
	seen    map[string]bool
	packets map[byte]int
	bytes   map[byte]int
}

func newTally() *tally {
	return &tally{seen: map[string]bool{}, packets: map[byte]int{}, bytes: map[byte]int{}}
}

func (t *tally) record(direction string, b []byte) {
	if len(b) < programblocks.PacketHeaderLength {
		return
	}
	key := fmt.Sprintf("%s %x", direction, b[1:6])
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.seen[key] {
		return
	}
	t.seen[key] = true
	op := b[1] &^ programblocks.ReplyFlag
	t.packets[op]++
	t.bytes[op] += len(b)
}

func (t *tally) total(ops ...byte) (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, n := 0, 0
	for op := range t.packets {
		if len(ops) == 0 || bytes.IndexByte(ops, op) >= 0 {
			p, n = p+t.packets[op], n+t.bytes[op]
		}
	}
	return p, n
}

type countedConn struct {
	net.Conn
	tally *tally
}

func (c countedConn) Write(b []byte) (int, error) {
	c.tally.record("request", b)
	return c.Conn.Write(b)
}

func (c countedConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if err == nil {
		c.tally.record("reply", b[:n])
	}
	return n, err
}

type countedPacketConn struct {
	net.PacketConn
	tally *tally
}

func (c countedPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.tally.record(c.LocalAddr().String()+" to "+addr.String(), b)
	return c.PacketConn.WriteTo(b, addr)
}

// fixture is the reference program served by a catalog.
type fixture struct {
	catalog  *programblocks.Catalog
	signer   cipher.PubKey
	secret   cipher.SecKey
	files    map[string][]byte
	address  programblocks.Hash
	encoded  []byte
	manifest *programblocks.Manifest
	peers    *programnetwork.PeerList
}

var date = time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

func newFixture(t *testing.T) *fixture {
	files, err := programblocks.ReadProgramDirectory("../../apps/benchmarks")
	if err != nil {
		t.Fatal(err)
	}
	pub, secret := cipher.MustGenerateDeterministicKeyPair([]byte("packet size sweep"))
	c, err := programblocks.NewCatalog(map[string]map[string][]byte{"benchmarks": files}, secret, date)
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{catalog: c, signer: pub, secret: secret, files: files}
	for _, a := range c.Addresses() {
		if a != c.ListAddress {
			f.address = a
		}
	}
	f.encoded, _, _ = c.Item(f.address)
	if f.manifest, _, err = programblocks.Decode(f.encoded, f.address); err != nil {
		t.Fatal(err)
	}
	// Five public IPv6 peers, 25-byte entries, and a private one that is
	// never sent.
	text := "[peers]\n[2001:db8::5]:7010, 1795478400, 1\n[2001:db8::6]:7010, 1795478400, 1\n[2001:db8::7]:7010, 1795478400, 1\n" +
		"[2001:db8::8]:7010, 1795478400, 1\n[2001:db8::9]:7010, 1795478400, 1\n[2001:db8::a]:7010, 1795478400, 0\n"
	if f.peers, err = programnetwork.ParsePeerList(text); err != nil {
		t.Fatal(err)
	}
	return f
}

// client serves the catalog at size and returns a client of size and forms.
func (f *fixture) client(t *testing.T, size int, forms programblocks.Forms) (*programblocks.Client, *tally) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go f.catalog.Serve(pc, size)
	conn, err := net.Dial("udp", pc.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(); pc.Close() })
	tl := newTally()
	c := programblocks.NewClient(countedConn{Conn: conn, tally: tl}, f.signer)
	c.PacketSize, c.Forms = size, forms
	return c, tl
}

func (f *fixture) node(t *testing.T, size int, forms programblocks.Forms, tl *tally) (*programnetwork.Node, string) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	st, err := programstore.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n, err := programnetwork.NewNode(countedPacketConn{PacketConn: pc, tally: tl}, st, []cipher.PubKey{f.signer})
	if err != nil {
		t.Fatal(err)
	}
	n.PacketSize, n.Forms = size, forms
	n.FirstTimeout, n.MaxTimeout, n.MaxTries = 250*time.Millisecond, 500*time.Millisecond, 3
	go n.Run()
	go n.RunUpdates(nil)
	t.Cleanup(func() { n.Close() })
	return n, pc.LocalAddr().String()
}

// announceResult is one have of the reference program from one node to a
// subscriber, and the subscriber's request and download.
type announceResult struct {
	tally *tally
	held  bool
}

func (f *fixture) announce(t *testing.T, size int, forms programblocks.Forms) announceResult {
	tl := newTally()
	publisher, publisherAddr := f.node(t, size, forms, tl)
	subscriber, _ := f.node(t, size, forms, tl)
	held := make(chan bool, 1)
	subscriber.OnHeld = func(programnetwork.Held) { held <- true }
	if err := subscriber.Subscribe(publisherAddr); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); publisher.Stats().Subscribers == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			return announceResult{tally: tl}
		}
	}
	if _, err := publisher.Publish(f.files, f.secret, date); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		select {
		case <-held:
			return announceResult{tally: tl, held: true}
		default:
		}
		if publisher.Stats().HavesUnacknowledged > 0 {
			return announceResult{tally: tl}
		}
	}
	return announceResult{tally: tl}
}

// operation is one row of the table. floor is the smallest packet size its
// layout allows, and reason why a smaller one fails.
type operation struct {
	name   string
	floor  int
	reason string
	run    func(t *testing.T, f *fixture, size int) (packets, bytes int, err error)
}

func clientRun(forms programblocks.Forms, fetch func(*programblocks.Client, *fixture) error) func(*testing.T, *fixture, int) (int, int, error) {
	return func(t *testing.T, f *fixture, size int) (int, int, error) {
		c, tl := f.client(t, size, forms)
		if err := fetch(c, f); err != nil {
			return 0, 0, err
		}
		p, b := tl.total()
		return p, b, nil
	}
}

var errNotHeld = errors.New("the subscriber did not get the program")

func announceRun(forms programblocks.Forms, ops ...byte) func(*testing.T, *fixture, int) (int, int, error) {
	return func(t *testing.T, f *fixture, size int) (int, int, error) {
		r := f.announceAt(t, size, forms)
		p, b := r.tally.total(ops...)
		if len(ops) == 1 && ops[0] == byte(programnetwork.OperationSubscribe) && p >= 2 {
			return p, b, nil
		}
		if !r.held {
			return 0, 0, errNotHeld
		}
		return p, b, nil
	}
}

// peersRun asks a node for its five public peers. A reply holds as many
// as fit, so at least one is wanted.
func peersRun() func(*testing.T, *fixture, int) (int, int, error) {
	return func(t *testing.T, f *fixture, size int) (int, int, error) {
		tl := newTally()
		server, serverAddr := f.node(t, size, full, tl)
		server.SetPeers(f.peers)
		asker, _ := f.node(t, size, full, tl)
		server.Now, asker.Now = func() time.Time { return date }, func() time.Time { return date }
		got, err := asker.GetPeers(serverAddr)
		if err != nil {
			return 0, 0, err
		}
		if want := min(5, (size-7)/25); len(got) != want {
			return 0, 0, fmt.Errorf("%d peers, want %d", len(got), want)
		}
		if len(got) == 0 {
			return 0, 0, programblocks.ErrPacketTooSmall
		}
		p, b := tl.total()
		return p, b, nil
	}
}

type announceKey struct {
	size  int
	forms programblocks.Forms
}

var announces = map[announceKey]announceResult{}

func (f *fixture) announceAt(t *testing.T, size int, forms programblocks.Forms) announceResult {
	if r, ok := announces[announceKey{size, forms}]; ok {
		return r
	}
	r := f.announce(t, size, forms)
	announces[announceKey{size, forms}] = r
	return r
}

var full, short = programblocks.FormsFull, programblocks.FormsShort

var operations = []operation{
	{"list manifests 0x01", 42, "reply: header 6, 32-byte address, length 4", clientRun(full, func(c *programblocks.Client, f *fixture) error {
		addr, _, err := c.ListManifests()
		if err == nil && addr != f.catalog.ListAddress {
			err = errors.New("wrong list address")
		}
		return err
	})},
	{"list manifests prefix 0x05", 15, "reply: header 6, prefix 1+4, length 4", clientRun(short, func(c *programblocks.Client, f *fixture) error {
		addr, n, _, err := c.ListManifestsPrefix()
		if err == nil && !f.catalog.ListAddress.HasPrefix(addr, n) {
			err = errors.New("wrong list prefix")
		}
		return err
	})},
	{"get manifest 0x02", 47, "request 44 with the 32-byte address, reply 46 before the first byte", clientRun(full, fetchManifest)},
	{"get manifest by prefix 0x06", 20, "reply: header 6, prefix 1+4, offset 4, length 4, one byte", clientRun(short, fetchManifest)},
	{"get block list 0x03", 92, "reply 44 with the 32-byte address, plus one 48-byte block entry", clientRun(full, fetchBlockList)},
	{"get block list bytes 0x07", 20, "as get manifest by prefix", clientRun(short, fetchBlockList)},
	{"get blocks 0x04", 560, "reply 48 with the 32-byte address, plus one whole 512-byte block", clientRun(full, fetchBlocks)},
	{"get block part 0x08", 16, "reply: header 6, block prefix 1+4, offset 2, length 2, one byte", clientRun(short, fetchBlocks)},
	{"fetch program, forms=full", 560, "get blocks", clientRun(full, fetchProgram)},
	{"fetch program, forms=short", 20, "get manifest by prefix", clientRun(short, fetchProgram)},
	{"subscribe 0x11", 10, "request and reply: header 6, lease 4", announceRun(short, byte(programnetwork.OperationSubscribe))},
	{"have 0x10", 50, "request: header 6, 32-byte address, lengths 4 and 8", announceRun(short, byte(programnetwork.OperationHave))},
	{"announce and request one program, forms=full", 560, "get blocks", announceRun(full)},
	{"announce and request one program, forms=short", 50, "the have", announceRun(short)},
	{"get peers 0x12", 32, "reply: header 6, count 1, one 25-byte IPv6 entry", peersRun()},
}

func fetchProgram(c *programblocks.Client, f *fixture) error {
	p, err := c.FetchProgram("benchmarks")
	if err == nil && p.Address != f.address {
		err = errors.New("wrong program")
	}
	return err
}

func fetchManifest(c *programblocks.Client, f *fixture) error {
	_, b, err := c.FetchManifest(f.address)
	if err == nil && !bytes.Equal(b, f.encoded) {
		err = errors.New("wrong manifest bytes")
	}
	return err
}

func fetchBlockList(c *programblocks.Client, f *fixture) error {
	got, err := c.FetchBlockEntries(f.address, 1, len(f.manifest.Blocks))
	if err == nil && !reflect.DeepEqual(got, f.manifest.Blocks) {
		err = errors.New("wrong block entries")
	}
	return err
}

func fetchBlocks(c *programblocks.Client, f *fixture) error {
	files, err := c.FetchFiles(f.address, f.manifest)
	if err == nil && !reflect.DeepEqual(files, f.files) {
		err = errors.New("wrong files")
	}
	return err
}

func TestPacketSizeSweep(t *testing.T) {
	f := newFixture(t)
	var table strings.Builder
	table.WriteString("| Operation |")
	for _, s := range sweepSizes {
		fmt.Fprintf(&table, " %d |", s)
	}
	table.WriteString(" Floor | Below the floor |\n|---|")
	for range sweepSizes {
		table.WriteString("---|")
	}
	table.WriteString("---|---|\n")
	for _, op := range operations {
		fmt.Fprintf(&table, "| %s |", op.name)
		for _, size := range sweepSizes {
			p, b, err := op.run(t, f, size)
			works := size >= op.floor
			switch {
			case works && err != nil:
				t.Errorf("%s at %d: %v, its floor is %d", op.name, size, err, op.floor)
			case !works && err == nil:
				t.Errorf("%s at %d works, below its floor %d", op.name, size, op.floor)
			case !works && !errors.Is(err, programblocks.ErrPacketTooSmall) && !errors.Is(err, errNotHeld):
				t.Errorf("%s at %d: %v, want a packet too small error", op.name, size, err)
			}
			if err != nil {
				table.WriteString(" fails |")
				continue
			}
			fmt.Fprintf(&table, " %d/%d |", p, b)
		}
		fmt.Fprintf(&table, " %d | %s |\n", op.floor, op.reason)
	}
	t.Logf("packets/bytes per fetch of apps/benchmarks (%d manifest bytes, %d blocks, %d packing bytes):\n%s",
		len(f.encoded), len(f.manifest.Blocks), f.manifest.PackLength, table.String())
	if *update {
		if err := os.WriteFile(tablePath, []byte(table.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(tablePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != table.String() {
		t.Errorf("the table differs from %s; rerun with -update and copy it into docs/ProgramDistribution.md", tablePath)
	}
}
