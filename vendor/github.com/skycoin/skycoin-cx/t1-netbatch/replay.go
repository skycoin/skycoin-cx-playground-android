package netbatch

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/skycoin/skycoin-cx/t0-textsections"
)

// NetReplayFormatVersion is the version of the replay record format.
const NetReplayFormatVersion = 1

// The replay record is a text sections file, see docs/Networking.md. The
// updates section has one line per update, `u number clock`, and after it
// one line per entry of its batch, `e handle trigger detail peer payload`.
// The peer is the UDP address of a data entry or -, and the payload is the
// quoted bytes of a data entry or -.

// netRecordWriter writes the record of a live run, one flushed update at a
// time, so an exit at any point leaves the updates before it.
type netRecordWriter struct {
	recordFile    *os.File
	recordWriter  *bufio.Writer
	recordLine    []byte
	recordPayload []byte
}

// recordOpen creates the record at path and writes its header.
func (record *netRecordWriter) recordOpen(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	record.recordFile = file
	record.recordWriter = bufio.NewWriterSize(file, 64<<10)
	record.recordLine = make([]byte, 0, 4*NetRingSize+256)
	record.recordPayload = make([]byte, NetRingSize)
	fmt.Fprintf(record.recordWriter, "# CX net replay record, see docs/Networking.md.\n[format] version\n%d\n\n[updates] kind fields\n", NetReplayFormatVersion)
	return record.recordWriter.Flush()
}

// recordClose flushes and closes the record.
func (record *netRecordWriter) recordClose() {
	if record.recordFile == nil {
		return
	}
	record.recordWriter.Flush()
	record.recordFile.Close()
	*record = netRecordWriter{}
}

// recordUpdate writes update number with its clock reading and entries.
func (record *netRecordWriter) recordUpdate(number int64, clock int64, entries []NetEntry) {
	line := append(record.recordLine[:0], "u "...)
	line = strconv.AppendInt(line, number, 10)
	line = append(line, ' ')
	line = strconv.AppendInt(line, clock, 10)
	line = append(line, '\n')
	record.recordWriter.Write(line)
	for i := range entries {
		entry := &entries[i]
		line = append(line[:0], "e "...)
		line = strconv.AppendInt(line, int64(entry.EntryHandle), 10)
		line = append(line, ' ')
		line = strconv.AppendInt(line, int64(entry.EntryTrigger), 10)
		line = append(line, ' ')
		line = strconv.AppendInt(line, int64(entry.EntryDetail), 10)
		line = append(line, ' ')
		if entry.entryAddress.IsValid() {
			line = entry.entryAddress.AppendTo(line)
		} else {
			line = append(line, '-')
		}
		line = append(line, ' ')
		if entry.EntryTrigger == NetTriggerData {
			payload := record.recordPayload[:entry.entryLength]
			netRingCopy(payload, entry.entrySocket.socketRing, int(entry.entryOffset))
			line = netPayloadQuote(line, payload)
		} else {
			line = append(line, '-')
		}
		line = append(line, '\n')
		record.recordWriter.Write(line)
	}
	record.recordLine = line
	record.recordWriter.Flush()
}

// netRingCopy copies len(destination) bytes of ring from start, wrapping.
func netRingCopy(destination []byte, ring []byte, start int) {
	copied := copy(destination, ring[start:])
	copy(destination[copied:], ring)
}

// netPayloadQuote appends payload as a Go quoted string, printable ASCII as
// it is and every other byte as \xNN, without a Go string conversion.
// VIOLATION STRHOT-348: B, writes the replay record of a recorded run, #508.
func netPayloadQuote(line []byte, payload []byte) []byte {
	const digits = "0123456789abcdef"
	line = append(line, '"')
	for _, b := range payload {
		switch {
		case b == '"' || b == '\\':
			line = append(line, '\\', b)
		case b >= 0x20 && b < 0x7f:
			line = append(line, b)
		default:
			line = append(line, '\\', 'x', digits[b>>4], digits[b&15])
		}
	}
	return append(line, '"')
}

// netReplayUpdate is one update of a loaded record.
type netReplayUpdate struct {
	updateClock int64
	updateFirst int
	updateCount int
}

// netReplay is a loaded record and the next update to show.
type netReplay struct {
	replayUpdates []netReplayUpdate
	replayEntries []NetEntry
	replayNext    int
	// replaySockets are made at load for the accepted entries, so a replayed
	// update allocates nothing.
	replaySockets []netSocket
	replayPath    string
}

// replayLoad reads the record at path.
func (replay *netReplay) replayLoad(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	*replay = netReplay{replayPath: path}
	sections, err := textsections.TextSectionsRead(content)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	format, found := textsections.TextSectionFind(sections, "format")
	if !found || len(format.TextSectionEntries) != 1 || format.TextSectionEntries[0].TextSectionEntryText != strconv.Itoa(NetReplayFormatVersion) {
		return fmt.Errorf("%s: not a net replay record of format %d", path, NetReplayFormatVersion)
	}
	updates, _ := textsections.TextSectionFind(sections, "updates")
	accepted := 0
	for _, line := range updates.TextSectionEntries {
		fail := func(problem string) error {
			return fmt.Errorf("%s:%d: %s: %q", path, line.TextSectionEntryLine, problem, line.TextSectionEntryText)
		}
		fields := strings.SplitN(line.TextSectionEntryText, " ", 6)
		switch {
		case fields[0] == "u" && len(fields) == 3:
			number, numberErr := strconv.Atoi(fields[1])
			clock, clockErr := strconv.ParseInt(fields[2], 10, 64)
			if numberErr != nil || clockErr != nil || number != len(replay.replayUpdates)+1 {
				return fail("bad update line")
			}
			replay.replayUpdates = append(replay.replayUpdates, netReplayUpdate{updateClock: clock, updateFirst: len(replay.replayEntries)})
		case fields[0] == "e" && len(fields) == 6 && len(replay.replayUpdates) > 0:
			var entry NetEntry
			var values [3]int64
			for i := range values {
				value, valueErr := strconv.ParseInt(fields[1+i], 10, 32)
				if valueErr != nil {
					return fail("bad entry number")
				}
				values[i] = value
			}
			entry.EntryHandle, entry.EntryTrigger, entry.EntryDetail = int32(values[0]), NetTrigger(values[1]), int32(values[2])
			if entry.EntryHandle <= 0 || entry.EntryHandle >= NetSocketCountMax || entry.EntryTrigger <= NetTriggerNone || entry.EntryTrigger >= NetTriggerCount {
				return fail("bad handle or trigger")
			}
			if fields[4] != "-" {
				address, addressErr := netip.ParseAddrPort(fields[4])
				if addressErr != nil {
					return fail("bad peer address")
				}
				entry.entryAddress = address
			}
			if fields[5] != "-" {
				payload, payloadErr := strconv.Unquote(fields[5])
				if payloadErr != nil || entry.EntryTrigger != NetTriggerData || len(payload) != int(entry.EntryDetail) {
					return fail("bad payload")
				}
				entry.entryPayload = []byte(payload)
				entry.entryLength = int32(len(payload))
			}
			if entry.EntryTrigger == NetTriggerAccepted {
				accepted++
			}
			replay.replayEntries = append(replay.replayEntries, entry)
			replay.replayUpdates[len(replay.replayUpdates)-1].updateCount++
		default:
			return fail("not an update or entry line")
		}
	}
	replay.replaySockets = make([]netSocket, accepted)
	for i := range replay.replaySockets {
		replay.replaySockets[i] = netSocket{socketKind: netSocketKindTCPStream, socketQuit: make(chan struct{})}
	}
	return nil
}

// replayUpdate shows the next update of the record and returns its entry
// count, or -1 when the record has no more updates.
func (replay *netReplay) replayUpdate() int32 {
	s := &netState
	s.netBatchCount, s.netBatchNext = 0, 0
	netSendFlush()
	if replay.replayNext >= len(replay.replayUpdates) {
		return -1
	}
	update := replay.replayUpdates[replay.replayNext]
	replay.replayNext++
	s.netClockMilliseconds = update.updateClock
	entries := replay.replayEntries[update.updateFirst : update.updateFirst+update.updateCount]
	for i := range entries {
		entry := entries[i]
		if entry.EntryTrigger == NetTriggerAccepted && entry.EntryDetail > 0 && entry.EntryDetail < NetSocketCountMax && s.netSockets[entry.EntryDetail] == nil && len(replay.replaySockets) > 0 {
			accepted := &replay.replaySockets[0]
			replay.replaySockets = replay.replaySockets[1:]
			accepted.socketHandle = entry.EntryDetail
			s.netSockets[entry.EntryDetail] = accepted
		}
		entry.entrySocket = s.netSockets[entry.EntryHandle]
		if entry.entrySocket == nil {
			// The record does not belong to this program.
			return -1
		}
		netEntryAdd(entry)
	}
	netBatchFinish()
	return int32(s.netBatchCount)
}

// NetReplayUpdatesLeft reports how many updates of the replay record are not
// shown yet.
func NetReplayUpdatesLeft() int {
	return len(netState.netReplay.replayUpdates) - netState.netReplay.replayNext
}
