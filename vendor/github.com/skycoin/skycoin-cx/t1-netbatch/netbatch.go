// Package netbatch is the runtime of the CX net library, see
// docs/Networking.md and skycoin-cx#508.
//
// Sockets are read by one goroutine each into a fixed receive ring, and every
// arrival is an event on one channel of fixed capacity. A program sees none
// of it until it calls NetUpdate, which drains the channel into the visible
// batch, sorts it by socket handle and hands the sends of the last batch to
// the writers. Between two updates the batch does not change. A live run can
// write every batch to a replay record, and a replay run reads the batches
// from the record and opens no socket, so it prints what the live run printed.
package netbatch

import (
	"net"
	"net/netip"
	"slices"
	"sync"
	"time"
)

// Fixed capacities. Each is allocated once, by NetPrepare or when a socket
// opens, so an update and a read allocate nothing.
const (
	// NetSocketCountMax sizes the handle table. Handles are 1 to
	// NetSocketCountMax-1, and 0 is no socket.
	NetSocketCountMax = 1024
	// NetArrivalCountMax is the capacity of the arrival channel. A TCP reader
	// waits when it is full, a UDP datagram is dropped and counted.
	NetArrivalCountMax = 4096
	// NetBatchCountMax is the most entries one update shows: the channel,
	// one listening or connected entry, one timeout and one dropped entry per
	// socket.
	NetBatchCountMax = NetArrivalCountMax + 3*NetSocketCountMax
	// NetRingSize is the receive ring of a socket. A full ring stops the TCP
	// reader until the next update frees the bytes of the batch before it.
	NetRingSize = 16 << 10
	// NetSendSize is what a program may write to one socket between two
	// updates.
	NetSendSize = 16 << 10
	// NetDatagramSizeMax is the largest UDP datagram a socket receives whole.
	NetDatagramSizeMax = 64 << 10
	// NetPeerCountMax sizes the UDP peer table. Peers are 1 to
	// NetPeerCountMax-1, and 0 is no peer.
	NetPeerCountMax = 1024
	// netDialTimeout bounds a TCP dial, which then posts an error entry.
	netDialTimeout = 30 * time.Second
)

// NetTrigger says why an entry is in the batch. The numbers are part of the
// language, net.TriggerData and so on.
type NetTrigger int32

const (
	// NetTriggerNone is the entry NetReadyNext returns after the last one.
	NetTriggerNone NetTrigger = iota
	// NetTriggerData is a received payload. The detail is its length.
	NetTriggerData
	// NetTriggerAccepted is a connection a TCP listener accepted. The detail
	// is the handle of the new connection.
	NetTriggerAccepted
	// NetTriggerConnected is a TCP dial that connected, or a UDP dial.
	NetTriggerConnected
	// NetTriggerListening is a listener that is bound. The detail is its
	// port.
	NetTriggerListening
	// NetTriggerClosed is a TCP connection the peer closed.
	NetTriggerClosed
	// NetTriggerError is a failed dial, listen, read or write. The detail is
	// a NetError.
	NetTriggerError
	// NetTriggerTimeout is a timeout NetTimeoutSet started that expired.
	NetTriggerTimeout
	// NetTriggerDropped counts the UDP datagrams dropped since the last
	// dropped entry of the socket, in the detail, because the ring or the
	// channel was full.
	NetTriggerDropped
	// NetTriggerCount is one past the last trigger.
	NetTriggerCount
)

// NetError is the detail of a NetTriggerError entry. The numbers are part of
// the language, net.ErrorRefused and so on.
type NetError int32

const (
	NetErrorNone NetError = iota
	// NetErrorOther is any error without its own number.
	NetErrorOther
	// NetErrorRefused is a dial the peer refused.
	NetErrorRefused
	// NetErrorReset is a connection the peer reset.
	NetErrorReset
	// NetErrorTimeout is a dial that did not connect in time.
	NetErrorTimeout
	// NetErrorAddress is an address that does not parse or resolve.
	NetErrorAddress
	// NetErrorAddressInUse is a listen on an address in use.
	NetErrorAddressInUse
	// NetErrorSocketTableFull is an accepted connection that found no free
	// handle. It is closed.
	NetErrorSocketTableFull
	// NetErrorCount is one past the last error.
	NetErrorCount
)

// NetMode says where the batches come from.
type NetMode int

const (
	// NetModeLive reads sockets.
	NetModeLive NetMode = iota
	// NetModeRecord reads sockets and writes every batch to the replay
	// record.
	NetModeRecord
	// NetModeReplay reads the batches from the replay record and opens no
	// socket.
	NetModeReplay
)

// netSocketKind is what a handle holds.
type netSocketKind uint8

const (
	netSocketKindNone netSocketKind = iota
	netSocketKindTCPListener
	netSocketKindTCPStream
	netSocketKindUDP
)

// netSocket is one open socket. It is made when the socket opens, so the
// goroutines that read and write it may outlive its handle, and a handle
// reused after a close never meets an event of the old socket.
type netSocket struct {
	socketKind   netSocketKind
	socketHandle int32

	socketStream   net.Conn
	socketListener net.Listener
	socketPacket   *net.UDPConn
	// socketQuit is closed when the program closes the handle. A goroutine
	// of the socket then posts nothing more.
	socketQuit chan struct{}

	// socketRing receives the payloads. socketRingWrite belongs to the
	// reader, socketRingUsed to netMutex. The bytes are freed in arrival
	// order, so the oldest are always the next to free.
	socketRing      []byte
	socketRingWrite int
	socketRingUsed  int
	// socketSpaceReady wakes a reader waiting for ring space.
	socketSpaceReady chan struct{}
	// socketDatagram receives one UDP datagram before it is copied into the
	// ring.
	socketDatagram []byte

	// socketVisibleBytes are the ring bytes of the visible batch, freed at
	// the next update. socketTouched lists the socket in netTouched once.
	socketVisibleBytes int
	socketTouched      bool

	// socketDropped counts UDP datagrams dropped since the last dropped
	// entry, under netMutex.
	socketDropped int32

	// The program writes socketSend and the writer goroutine socketFlush.
	// An update swaps them when the writer holds the idle token.
	socketSend        []byte
	socketSendLength  int
	socketFlush       []byte
	socketWriterWake  chan struct{}
	socketWriterIdle  chan struct{}
	socketWriterStart bool
	socketClosing     bool
	socketSendPending bool

	// socketConnectedVisible is set when the program has seen the socket
	// connected, accepted or bound. Writes before it are refused.
	socketConnectedVisible bool
	socketClosedByProgram  bool
}

// netArrival is one event a goroutine posts.
type netArrival struct {
	arrivalSocket   *netSocket
	arrivalTrigger  NetTrigger
	arrivalDetail   int32
	arrivalOffset   int32
	arrivalLength   int32
	arrivalPeer     netip.AddrPort
	arrivalAccepted *netSocket
}

// NetEntry is one entry of the visible batch.
type NetEntry struct {
	EntryHandle  int32
	EntryTrigger NetTrigger
	EntryDetail  int32
	// EntryPeer is the UDP peer of a data entry, 0 otherwise.
	EntryPeer int32

	entrySocket  *netSocket
	entryOffset  int32
	entryLength  int32
	entryPayload []byte
	entryAddress netip.AddrPort
}

// NetState is the whole net runtime. One exists per process.
type NetState struct {
	netMutex sync.Mutex
	netMode  NetMode

	netSockets  [NetSocketCountMax]*netSocket
	netArrivals chan netArrival
	// netLocal are events the program side posts itself, a listen or UDP
	// dial result, which must not wait on the channel it drains.
	netLocal      [NetSocketCountMax]netArrival
	netLocalCount int

	netBatch      [NetBatchCountMax]NetEntry
	netBatchCount int
	netBatchNext  int

	netTouched      [NetBatchCountMax]*netSocket
	netTouchedCount int
	netDroppedList  [NetSocketCountMax]*netSocket
	netDroppedCount int
	netSendList     [NetSocketCountMax]int32
	netSendCount    int

	netClockStart        time.Time
	netClockMilliseconds int64
	netUpdateNumber      int64
	netWaitTimer         *time.Timer

	netTimers NetTimerWheel
	netPeers  NetPeerTable

	netRecord netRecordWriter
	netReplay netReplay

	// NetUpdateWork counts the entries and sockets each update touched, for
	// the test that the work grows with the ready count and not with the
	// socket count.
	NetUpdateWork int64
}

// VIOLATION: state outside State. Owner: t1-netbatch; moves to a per program state with the cxfx and cipher tables, see D45.
var netState NetState

// NetPrepare allocates the fixed state once, before any run.
func NetPrepare() {
	s := &netState
	if s.netArrivals != nil {
		return
	}
	s.netArrivals = make(chan netArrival, NetArrivalCountMax)
	s.netWaitTimer = time.NewTimer(time.Hour)
	s.netWaitTimer.Stop()
	s.netTimers.timerWheelClear()
	s.netPeers.peerTableClear()
}

// NetRunStart closes what an earlier run left open and starts a run in
// mode. recordPath names the replay record to write or read.
func NetRunStart(mode NetMode, recordPath string) error {
	NetPrepare()
	NetRunStop()
	s := &netState
	s.netMode = mode
	s.netClockStart = time.Now()
	switch mode {
	case NetModeRecord:
		return s.netRecord.recordOpen(recordPath)
	case NetModeReplay:
		return s.netReplay.replayLoad(recordPath)
	}
	return nil
}

// NetRunStop closes every socket, flushes the replay record and empties the
// batch.
func NetRunStop() {
	s := &netState
	if s.netArrivals == nil {
		return
	}
	for handle := range s.netSockets {
		if s.netSockets[handle] != nil {
			netSocketClose(int32(handle), false)
		}
	}
	for len(s.netArrivals) > 0 {
		arrival := <-s.netArrivals
		if accepted := arrival.arrivalAccepted; accepted != nil {
			close(accepted.socketQuit)
			close(accepted.socketWriterWake)
			accepted.socketStream.Close()
		}
	}
	s.netRecord.recordClose()
	s.netReplay = netReplay{}
	s.netLocalCount, s.netBatchCount, s.netBatchNext = 0, 0, 0
	s.netTouchedCount, s.netDroppedCount, s.netSendCount = 0, 0, 0
	s.netClockMilliseconds, s.netUpdateNumber = 0, 0
	s.netTimers.timerWheelClear()
	s.netPeers.peerTableClear()
	s.netMode = NetModeLive
}

// NetHandleValid reports whether handle names an open socket.
func NetHandleValid(handle int32) bool {
	return handle > 0 && handle < NetSocketCountMax && netState.netSockets[handle] != nil
}

// netHandleTake returns the lowest free handle, or 0 when the table is full.
// Only the program takes and frees handles, so the numbers are a function of
// the program and its batches.
func netHandleTake() int32 {
	for handle := int32(1); handle < NetSocketCountMax; handle++ {
		if netState.netSockets[handle] == nil {
			return handle
		}
	}
	return 0
}

// netSocketNew makes a socket of kind. A replay socket has no buffers.
func netSocketNew(kind netSocketKind) *netSocket {
	socket := &netSocket{socketKind: kind, socketQuit: make(chan struct{})}
	if netState.netMode == NetModeReplay || kind == netSocketKindTCPListener {
		return socket
	}
	socket.socketRing = make([]byte, NetRingSize)
	socket.socketSpaceReady = make(chan struct{}, 1)
	socket.socketSend = make([]byte, 0, NetSendSize)
	socket.socketFlush = make([]byte, 0, NetSendSize)
	socket.socketWriterWake = make(chan struct{}, 1)
	socket.socketWriterIdle = make(chan struct{}, 1)
	if kind == netSocketKindUDP {
		socket.socketDatagram = make([]byte, NetDatagramSizeMax)
	}
	return socket
}

// netLocalPost adds an event the program side made itself.
func netLocalPost(arrival netArrival) {
	s := &netState
	s.netLocal[s.netLocalCount] = arrival
	s.netLocalCount++
}

// netArrivalSend posts arrival and reports false when the program closed the
// socket first.
func netArrivalSend(socket *netSocket, arrival netArrival) bool {
	select {
	case netState.netArrivals <- arrival:
		return true
	case <-socket.socketQuit:
		return false
	}
}

// NetClock returns the clock reading of the last update in milliseconds
// since the run started. A replay returns the recorded reading.
func NetClock() int64 {
	return netState.netClockMilliseconds
}

// NetReadyCount returns how many entries of the batch NetReadyNext has not
// returned yet.
func NetReadyCount() int32 {
	return int32(netState.netBatchCount - netState.netBatchNext)
}

// NetReadyNext returns the next entry of the batch and its message id, the
// index of the entry in the batch, or NetTriggerNone after the last one.
func NetReadyNext() (handle int32, trigger NetTrigger, messageID int32, detail int32) {
	s := &netState
	if s.netBatchNext >= s.netBatchCount {
		return 0, NetTriggerNone, 0, 0
	}
	messageID = int32(s.netBatchNext)
	entry := &s.netBatch[messageID]
	s.netBatchNext++
	return entry.EntryHandle, entry.EntryTrigger, messageID, entry.EntryDetail
}

// netDataEntry returns the data entry messageID of the batch when it belongs
// to the open socket handle.
func netDataEntry(handle int32, messageID int32) *NetEntry {
	s := &netState
	if messageID < 0 || int(messageID) >= s.netBatchCount {
		return nil
	}
	entry := &s.netBatch[messageID]
	if entry.EntryTrigger != NetTriggerData || entry.EntryHandle != handle || !NetHandleValid(handle) || s.netSockets[handle] != entry.entrySocket {
		return nil
	}
	return entry
}

// NetRead copies the payload of data entry messageID of handle, from byte
// offset on, into destination and returns the bytes copied. It returns 0 for
// an entry that is not a data entry of the open socket handle, and for an
// offset at or past the end of the payload.
func NetRead(handle int32, messageID int32, destination []byte, offset int32) int32 {
	entry := netDataEntry(handle, messageID)
	if entry == nil || offset < 0 || offset >= entry.entryLength {
		return 0
	}
	if entry.entryPayload != nil {
		return int32(copy(destination, entry.entryPayload[offset:]))
	}
	ring := entry.entrySocket.socketRing
	start := (int(entry.entryOffset) + int(offset)) % len(ring)
	remaining := int(entry.entryLength - offset)
	copied := copy(destination[:min(len(destination), remaining)], ring[start:min(len(ring), start+remaining)])
	if copied < remaining && copied < len(destination) {
		copied += copy(destination[copied:min(len(destination), remaining)], ring)
	}
	return int32(copied)
}

// NetUDPReadFrom is NetRead for a UDP socket, and also returns the peer the
// datagram came from.
func NetUDPReadFrom(handle int32, messageID int32, destination []byte, offset int32) (int32, int32) {
	entry := netDataEntry(handle, messageID)
	if entry == nil {
		return 0, 0
	}
	return NetRead(handle, messageID, destination, offset), entry.EntryPeer
}

// NetWrite queues source on handle for the next update and returns the
// bytes queued, fewer than len(source) when the send buffer of the update is
// full. It queues nothing before the program has seen the socket connected
// or accepted.
func NetWrite(handle int32, source []byte) int32 {
	if !NetHandleValid(handle) {
		return 0
	}
	socket := netState.netSockets[handle]
	if socket.socketKind != netSocketKindTCPStream && socket.socketKind != netSocketKindUDP || !socket.socketConnectedVisible {
		return 0
	}
	if socket.socketKind == netSocketKindUDP {
		return netDatagramQueue(socket, netip.AddrPort{}, source)
	}
	queued := min(len(source), NetSendSize-socket.socketSendLength)
	if socket.socketSend != nil {
		socket.socketSend = append(socket.socketSend, source[:queued]...)
	}
	socket.socketSendLength += queued
	netSendMark(handle, socket)
	return int32(queued)
}

// NetUDPWriteTo queues source as one datagram to peer and returns its length,
// or 0 when it does not fit in the send buffer of the update or peer is not
// in the peer table.
func NetUDPWriteTo(handle int32, peer int32, source []byte) int32 {
	if !NetHandleValid(handle) {
		return 0
	}
	socket := netState.netSockets[handle]
	address, found := netState.netPeers.PeerAddress(peer)
	if socket.socketKind != netSocketKindUDP || !socket.socketConnectedVisible || !found {
		return 0
	}
	return netDatagramQueue(socket, address, source)
}

// netDatagramHeaderSize frames a queued datagram: the 16 byte address, the
// port and the length.
const netDatagramHeaderSize = 16 + 2 + 2

// netDatagramQueue frames source to address, the zero address for the peer
// of a dialed socket.
func netDatagramQueue(socket *netSocket, address netip.AddrPort, source []byte) int32 {
	if len(source) > NetDatagramSizeMax-1 || socket.socketSendLength+netDatagramHeaderSize+len(source) > NetSendSize {
		return 0
	}
	if socket.socketSend != nil {
		var header [netDatagramHeaderSize]byte
		addressBytes := address.Addr().As16()
		copy(header[:16], addressBytes[:])
		header[16], header[17] = byte(address.Port()>>8), byte(address.Port())
		header[18], header[19] = byte(len(source)>>8), byte(len(source))
		socket.socketSend = append(append(socket.socketSend, header[:]...), source...)
	}
	socket.socketSendLength += netDatagramHeaderSize + len(source)
	netSendMark(socket.socketHandle, socket)
	return int32(len(source))
}

// netSendMark lists handle for the next update to flush.
func netSendMark(handle int32, socket *netSocket) {
	if socket.socketSendPending {
		return
	}
	socket.socketSendPending = true
	netState.netSendList[netState.netSendCount] = handle
	netState.netSendCount++
}

// NetClose closes handle and frees it. What the program wrote since the last
// update is still sent before the socket closes. It reports false for a
// handle that is not open.
func NetClose(handle int32) bool {
	return netSocketClose(handle, true)
}

// netSocketClose is NetClose. A close that is not graceful drops what the
// program queued and does not wait for the writer, as a run stops.
func netSocketClose(handle int32, graceful bool) bool {
	if !NetHandleValid(handle) {
		return false
	}
	s := &netState
	socket := s.netSockets[handle]
	s.netSockets[handle] = nil
	s.netTimers.TimerStop(handle)
	s.netMutex.Lock()
	socket.socketClosedByProgram = true
	writerStarted := socket.socketWriterStart
	s.netMutex.Unlock()
	close(socket.socketQuit)
	if s.netMode == NetModeReplay {
		return true
	}
	if socket.socketListener != nil {
		socket.socketListener.Close()
	}
	if writerStarted && graceful {
		netWriterHandoff(socket, true)
		return true
	}
	if writerStarted {
		close(socket.socketWriterWake)
	}
	s.netMutex.Lock()
	stream, packet := socket.socketStream, socket.socketPacket
	s.netMutex.Unlock()
	if stream != nil {
		stream.Close()
	}
	if packet != nil {
		packet.Close()
	}
	return true
}

// NetTimeoutSet starts a timeout on handle that expires milliseconds after
// the clock reading of the last update, as a timeout entry of the first
// update at or after it. A running timeout of the handle is replaced, and 0
// stops it. It reports false for a handle that is not open.
func NetTimeoutSet(handle int32, milliseconds int32) bool {
	if !NetHandleValid(handle) || milliseconds < 0 {
		return false
	}
	s := &netState
	s.netTimers.TimerStop(handle)
	if milliseconds > 0 {
		s.netTimers.TimerStart(handle, s.netClockMilliseconds+int64(milliseconds))
	}
	return true
}

// NetUpdate frees the ring bytes of the batch before, hands the writes since
// then to the writers, waits up to waitMilliseconds when nothing has arrived,
// and makes everything that arrived the new batch, sorted by socket handle
// and in arrival order within a socket. It returns the entry count.
func NetUpdate(waitMilliseconds int32) int32 {
	s := &netState
	s.netUpdateNumber++
	if s.netMode == NetModeReplay {
		return s.netReplay.replayUpdate()
	}
	s.netBatchCount, s.netBatchNext = 0, 0
	netTouchedFree()
	netSendFlush()

	s.netMutex.Lock()
	dropped := s.netDroppedCount
	s.netMutex.Unlock()
	if waitMilliseconds > 0 && s.netLocalCount == 0 && len(s.netArrivals) == 0 && dropped == 0 {
		s.netWaitTimer.Reset(time.Duration(waitMilliseconds) * time.Millisecond)
		select {
		case arrival := <-s.netArrivals:
			netArrivalAdd(arrival)
		case <-s.netWaitTimer.C:
		}
		s.netWaitTimer.Stop()
	}
	s.netClockMilliseconds = time.Since(s.netClockStart).Milliseconds()

	for i := 0; i < s.netLocalCount; i++ {
		netArrivalAdd(s.netLocal[i])
	}
	s.netLocalCount = 0
	for count := len(s.netArrivals); count > 0; count-- {
		netArrivalAdd(<-s.netArrivals)
	}
	netAcceptedBind()

	s.netMutex.Lock()
	for i := 0; i < s.netDroppedCount; i++ {
		socket := s.netDroppedList[i]
		if !socket.socketClosedByProgram {
			netEntryAdd(NetEntry{EntryHandle: socket.socketHandle, EntryTrigger: NetTriggerDropped, EntryDetail: socket.socketDropped, entrySocket: socket})
		}
		socket.socketDropped = 0
		s.netDroppedList[i] = nil
	}
	s.netDroppedCount = 0
	s.netMutex.Unlock()

	s.netTimers.TimerAdvance(s.netClockMilliseconds, netTimeoutFire)
	netBatchFinish()
	if s.netMode == NetModeRecord {
		s.netRecord.recordUpdate(s.netUpdateNumber, s.netClockMilliseconds, s.netBatch[:s.netBatchCount])
	}
	return int32(s.netBatchCount)
}

// netBatchFinish sorts the batch by handle, stable so a socket keeps its
// arrival order, and lets the program see the sockets the batch connects.
func netBatchFinish() {
	s := &netState
	batch := s.netBatch[:s.netBatchCount]
	slices.SortStableFunc(batch, netEntryCompare)
	for i := range batch {
		entry := &batch[i]
		switch entry.EntryTrigger {
		case NetTriggerConnected, NetTriggerListening:
			entry.entrySocket.socketConnectedVisible = true
		case NetTriggerAccepted:
			if NetHandleValid(entry.EntryDetail) {
				s.netSockets[entry.EntryDetail].socketConnectedVisible = true
			}
		case NetTriggerData:
			if entry.entrySocket.socketKind == netSocketKindUDP {
				entry.EntryPeer = s.netPeers.PeerFind(entry.entryAddress, true)
			}
		}
	}
	s.NetUpdateWork += int64(len(batch))
}

// netEntryCompare orders entries by handle.
func netEntryCompare(a, b NetEntry) int {
	return int(a.EntryHandle) - int(b.EntryHandle)
}

// netEntryAdd appends entry to the batch.
func netEntryAdd(entry NetEntry) {
	s := &netState
	s.netBatch[s.netBatchCount] = entry
	s.netBatchCount++
}

// netArrivalAdd makes arrival an entry, unless the program closed its socket.
// The ring bytes of a data entry are freed at the next update.
func netArrivalAdd(arrival netArrival) {
	socket := arrival.arrivalSocket
	if socket.socketClosedByProgram {
		return
	}
	entry := NetEntry{
		EntryHandle:  socket.socketHandle,
		EntryTrigger: arrival.arrivalTrigger,
		EntryDetail:  arrival.arrivalDetail,
		entrySocket:  socket,
		entryOffset:  arrival.arrivalOffset,
		entryLength:  arrival.arrivalLength,
		entryAddress: arrival.arrivalPeer,
	}
	switch arrival.arrivalTrigger {
	case NetTriggerData:
		entry.EntryDetail = arrival.arrivalLength
		socket.socketVisibleBytes += int(arrival.arrivalLength)
		if !socket.socketTouched {
			socket.socketTouched = true
			netState.netTouched[netState.netTouchedCount] = socket
			netState.netTouchedCount++
		}
	case NetTriggerAccepted:
		entry.entrySocket = arrival.arrivalAccepted
	}
	netEntryAdd(entry)
}

// netAcceptedBind gives each accepted connection of the batch the lowest
// free handle, in the order of the listener handles and then of acceptance,
// so the handles depend only on what each listener accepted. A connection
// that finds no handle is closed and its entry is an error of the listener.
func netAcceptedBind() {
	s := &netState
	batch := s.netBatch[:s.netBatchCount]
	slices.SortStableFunc(batch, netEntryCompare)
	for i := range batch {
		entry := &batch[i]
		if entry.EntryTrigger != NetTriggerAccepted {
			continue
		}
		accepted := entry.entrySocket
		entry.entrySocket = s.netSockets[entry.EntryHandle]
		handle := netHandleTake()
		if handle == 0 {
			s.netMutex.Lock()
			accepted.socketClosedByProgram = true
			s.netMutex.Unlock()
			close(accepted.socketQuit)
			close(accepted.socketWriterWake)
			accepted.socketStream.Close()
			entry.EntryTrigger, entry.EntryDetail = NetTriggerError, int32(NetErrorSocketTableFull)
			continue
		}
		accepted.socketHandle = handle
		s.netSockets[handle] = accepted
		entry.EntryDetail = handle
	}
	// The data and close entries of a connection accepted in this batch
	// were added before it had a handle. Those of a connection that found
	// none are dropped.
	kept := 0
	for i := range batch {
		entry := batch[i]
		if entry.EntryHandle == 0 {
			entry.EntryHandle = entry.entrySocket.socketHandle
		}
		if entry.EntryHandle != 0 {
			batch[kept] = entry
			kept++
		}
	}
	s.netBatchCount = kept
	s.NetUpdateWork += int64(len(batch))
}

// netTouchedFree frees the ring bytes of the batch before and wakes the
// readers that wait for them.
func netTouchedFree() {
	s := &netState
	if s.netTouchedCount == 0 {
		return
	}
	s.netMutex.Lock()
	for i := 0; i < s.netTouchedCount; i++ {
		socket := s.netTouched[i]
		socket.socketRingUsed -= socket.socketVisibleBytes
		socket.socketVisibleBytes = 0
		socket.socketTouched = false
		s.netTouched[i] = nil
		select {
		case socket.socketSpaceReady <- struct{}{}:
		default:
		}
	}
	s.NetUpdateWork += int64(s.netTouchedCount)
	s.netTouchedCount = 0
	s.netMutex.Unlock()
}

// netSendFlush hands what each socket queued since the last update to its
// writer. It waits for a writer still sending the update before, which only
// a peer that stopped reading makes slow.
func netSendFlush() {
	s := &netState
	for i := 0; i < s.netSendCount; i++ {
		handle := s.netSendList[i]
		socket := s.netSockets[handle]
		if socket == nil {
			continue
		}
		socket.socketSendPending = false
		if s.netMode != NetModeReplay {
			netWriterHandoff(socket, false)
		}
		socket.socketSendLength = 0
	}
	s.NetUpdateWork += int64(s.netSendCount)
	s.netSendCount = 0
}

// netTimeoutFire adds the timeout entry of handle.
func netTimeoutFire(handle int32) {
	socket := netState.netSockets[handle]
	if socket != nil {
		netEntryAdd(NetEntry{EntryHandle: handle, EntryTrigger: NetTriggerTimeout, entrySocket: socket})
	}
}

// NetUpdateNumber returns the number of the last update, counted from 1.
func NetUpdateNumber() int64 {
	return netState.netUpdateNumber
}

// NetArrivalsPending returns how many events wait on the arrival channel for
// the next update.
func NetArrivalsPending() int {
	return len(netState.netArrivals)
}
