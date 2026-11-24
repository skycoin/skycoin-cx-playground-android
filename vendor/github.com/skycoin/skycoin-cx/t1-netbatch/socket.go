package netbatch

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
)

// NetTCPListen opens a TCP listener on address and returns its handle, or
// 0 when the handle table is full. The bind result is the listening entry,
// with the port, or an error entry of the next update.
// VIOLATION VMALLOC-018: a system library call allocates in Go and the OS, #407.
// VIOLATION STRHOT-339: B, hands an address to Go and the OS, #508.
func NetTCPListen(address string) int32 {
	handle := netHandleTake()
	if handle == 0 {
		return 0
	}
	socket := netSocketNew(netSocketKindTCPListener)
	socket.socketHandle = handle
	netState.netSockets[handle] = socket
	if netState.netMode == NetModeReplay {
		return handle
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		netLocalPost(netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerError, arrivalDetail: int32(netErrorOf(err))})
		return handle
	}
	socket.socketListener = listener
	netLocalPost(netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerListening, arrivalDetail: int32(netPortOf(listener.Addr()))})
	go netTCPAcceptLoop(socket)
	return handle
}

// NetTCPDial starts a TCP connection to address and returns its handle, or 0
// when the handle table is full. The result is the connected entry or an
// error entry of a later update.
// VIOLATION VMALLOC-018: a system library call allocates in Go and the OS, #407.
// VIOLATION STRHOT-340: B, hands an address to Go and the OS, #508.
func NetTCPDial(address string) int32 {
	handle := netHandleTake()
	if handle == 0 {
		return 0
	}
	socket := netSocketNew(netSocketKindTCPStream)
	socket.socketHandle = handle
	netState.netSockets[handle] = socket
	if netState.netMode == NetModeReplay {
		return handle
	}
	go netTCPDialRun(socket, address)
	return handle
}

// NetUDPListen opens a UDP socket bound to address and returns its handle, or
// 0 when the handle table is full. The bind result is the listening entry or
// an error entry of the next update.
// VIOLATION VMALLOC-018: a system library call allocates in Go and the OS, #407.
// VIOLATION STRHOT-341: B, hands an address to Go and the OS, #508.
func NetUDPListen(address string) int32 {
	return netUDPOpen(address, false)
}

// NetUDPDial opens a UDP socket whose writes go to address, through NetWrite,
// and returns its handle, or 0 when the handle table is full. The result is
// the connected entry or an error entry of the next update.
// VIOLATION VMALLOC-018: a system library call allocates in Go and the OS, #407.
// VIOLATION STRHOT-342: B, hands an address to Go and the OS, #508.
func NetUDPDial(address string) int32 {
	return netUDPOpen(address, true)
}

// netUDPOpen is NetUDPListen and NetUDPDial.
// VIOLATION STRHOT-343: B, hands an address to Go and the OS, #508.
func netUDPOpen(address string, dial bool) int32 {
	handle := netHandleTake()
	if handle == 0 {
		return 0
	}
	socket := netSocketNew(netSocketKindUDP)
	socket.socketHandle = handle
	netState.netSockets[handle] = socket
	if netState.netMode == NetModeReplay {
		return handle
	}
	var packet *net.UDPConn
	var err error
	if dial {
		var remote *net.UDPAddr
		if remote, err = net.ResolveUDPAddr("udp", address); err == nil {
			packet, err = net.DialUDP("udp", nil, remote)
		}
	} else {
		var local *net.UDPAddr
		if local, err = net.ResolveUDPAddr("udp", address); err == nil {
			packet, err = net.ListenUDP("udp", local)
		}
	}
	if err != nil {
		netLocalPost(netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerError, arrivalDetail: int32(netErrorOf(err))})
		return handle
	}
	socket.socketPacket = packet
	netWriterStart(socket)
	trigger, detail := NetTriggerConnected, int32(0)
	if !dial {
		trigger, detail = NetTriggerListening, int32(netPortOf(packet.LocalAddr()))
	}
	netLocalPost(netArrival{arrivalSocket: socket, arrivalTrigger: trigger, arrivalDetail: detail})
	go netUDPReadLoop(socket)
	return handle
}

// NetUDPPeerResolve returns the peer of the numeric address, such as
// 127.0.0.1:9000, adding it to the peer table, or 0 when it does not parse or
// the table is full.
// VIOLATION STRHOT-344: B, parses an address, #508.
func NetUDPPeerResolve(address string) int32 {
	parsed, err := netip.ParseAddrPort(address)
	if err != nil {
		return 0
	}
	return netState.netPeers.PeerFind(netip.AddrPortFrom(parsed.Addr().Unmap(), parsed.Port()), true)
}

// netPortOf returns the port of a bound address.
func netPortOf(address net.Addr) int {
	switch bound := address.(type) {
	case *net.TCPAddr:
		return bound.Port
	case *net.UDPAddr:
		return bound.Port
	}
	return 0
}

// netErrorOf returns the NetError of err.
// VIOLATION STRHOT-345: B, reads the error Go returned, outside the update, #508.
func netErrorOf(err error) NetError {
	var addressError *net.AddrError
	var dnsError *net.DNSError
	var parseError *net.ParseError
	switch {
	case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return NetErrorTimeout
	case errors.As(err, &addressError), errors.As(err, &dnsError), errors.As(err, &parseError):
		return NetErrorAddress
	case errors.Is(err, netErrnoRefused):
		return NetErrorRefused
	case errors.Is(err, netErrnoReset), errors.Is(err, netErrnoBrokenPipe):
		return NetErrorReset
	case errors.Is(err, netErrnoAddressInUse):
		return NetErrorAddressInUse
	}
	return NetErrorOther
}

// netTCPDialRun connects socket, then reads it. The writer starts before
// the connected entry is posted, so it runs when the program can write.
// VIOLATION STRHOT-347: B, hands an address to Go and the OS, in the dial goroutine, #508.
func netTCPDialRun(socket *netSocket, address string) {
	dialer := net.Dialer{Timeout: netDialTimeout}
	stream, err := dialer.Dial("tcp", address)
	if err != nil {
		netArrivalSend(socket, netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerError, arrivalDetail: int32(netErrorOf(err))})
		return
	}
	netState.netMutex.Lock()
	if socket.socketClosedByProgram {
		netState.netMutex.Unlock()
		stream.Close()
		return
	}
	socket.socketStream = stream
	netState.netMutex.Unlock()
	if !netWriterStart(socket) {
		stream.Close()
		return
	}
	if !netArrivalSend(socket, netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerConnected}) {
		return
	}
	netTCPReadLoop(socket)
}

// netTCPAcceptLoop accepts connections until the listener closes. Each
// accepted entry is posted before the reader of the connection starts, so
// it comes first in the arrival order.
func netTCPAcceptLoop(listener *netSocket) {
	for {
		stream, err := listener.socketListener.Accept()
		if err != nil {
			netArrivalSend(listener, netArrival{arrivalSocket: listener, arrivalTrigger: NetTriggerError, arrivalDetail: int32(netErrorOf(err))})
			return
		}
		accepted := netSocketNew(netSocketKindTCPStream)
		accepted.socketStream = stream
		netWriterStart(accepted)
		if !netArrivalSend(listener, netArrival{arrivalSocket: listener, arrivalTrigger: NetTriggerAccepted, arrivalAccepted: accepted}) {
			stream.Close()
			return
		}
		go netTCPReadLoop(accepted)
	}
}

// netTCPReadLoop reads socket into its ring until it closes. A full ring
// stops it until an update frees bytes, and TCP then slows the peer.
// VIOLATION STRHOT-338: B, compares the error of a socket read, in the reader goroutine of the socket and not the run loop, #508.
func netTCPReadLoop(socket *netSocket) {
	s := &netState
	for {
		s.netMutex.Lock()
		for socket.socketRingUsed == len(socket.socketRing) {
			s.netMutex.Unlock()
			select {
			case <-socket.socketSpaceReady:
			case <-socket.socketQuit:
				return
			}
			s.netMutex.Lock()
		}
		start := socket.socketRingWrite
		span := min(len(socket.socketRing)-socket.socketRingUsed, len(socket.socketRing)-start)
		s.netMutex.Unlock()
		read, err := socket.socketStream.Read(socket.socketRing[start : start+span])
		if read > 0 {
			s.netMutex.Lock()
			socket.socketRingUsed += read
			s.netMutex.Unlock()
			socket.socketRingWrite = (start + read) % len(socket.socketRing)
			if !netArrivalSend(socket, netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerData, arrivalOffset: int32(start), arrivalLength: int32(read)}) {
				return
			}
		}
		if err != nil {
			arrival := netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerClosed}
			if !errors.Is(err, io.EOF) {
				arrival.arrivalTrigger, arrival.arrivalDetail = NetTriggerError, int32(netErrorOf(err))
			}
			netArrivalSend(socket, arrival)
			return
		}
	}
}

// netUDPReadLoop reads datagrams into the ring until the socket closes. A
// datagram that does not fit in the ring or the channel is dropped and
// counted for the dropped entry of the next update.
// VIOLATION STRHOT-335: B, compares the error of a socket read, in the reader goroutine of the socket and not the run loop, #508.
func netUDPReadLoop(socket *netSocket) {
	s := &netState
	ring := socket.socketRing
	for {
		read, peer, err := socket.socketPacket.ReadFromUDPAddrPort(socket.socketDatagram)
		if err != nil {
			select {
			case <-socket.socketQuit:
				return
			default:
			}
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// A dialed UDP socket reads the ICMP error of an earlier write.
			if !netArrivalSend(socket, netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerError, arrivalDetail: int32(netErrorOf(err))}) {
				return
			}
			continue
		}
		s.netMutex.Lock()
		if read > len(ring)-socket.socketRingUsed {
			netDroppedCount(socket)
			s.netMutex.Unlock()
			continue
		}
		start := socket.socketRingWrite
		copied := copy(ring[start:], socket.socketDatagram[:read])
		copy(ring, socket.socketDatagram[copied:read])
		socket.socketRingUsed += read
		s.netMutex.Unlock()
		arrival := netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerData, arrivalOffset: int32(start), arrivalLength: int32(read), arrivalPeer: netip.AddrPortFrom(peer.Addr().Unmap(), peer.Port())}
		select {
		case s.netArrivals <- arrival:
			socket.socketRingWrite = (start + read) % len(ring)
		case <-socket.socketQuit:
			return
		default:
			s.netMutex.Lock()
			socket.socketRingUsed -= read
			netDroppedCount(socket)
			s.netMutex.Unlock()
		}
	}
}

// netDroppedCount counts a dropped datagram of socket, under netMutex.
func netDroppedCount(socket *netSocket) {
	s := &netState
	socket.socketDropped++
	if socket.socketDropped == 1 {
		s.netDroppedList[s.netDroppedCount] = socket
		s.netDroppedCount++
	}
}

// netWriterStart starts the writer of socket, idle, and reports false when
// the program closed the socket first.
func netWriterStart(socket *netSocket) bool {
	netState.netMutex.Lock()
	if socket.socketClosedByProgram {
		netState.netMutex.Unlock()
		return false
	}
	socket.socketWriterStart = true
	netState.netMutex.Unlock()
	socket.socketWriterIdle <- struct{}{}
	go netWriteLoop(socket)
	return true
}

// netWriterHandoff waits for the writer of socket to be idle and hands it
// what the program queued. With closing the writer closes the socket after.
func netWriterHandoff(socket *netSocket, closing bool) {
	<-socket.socketWriterIdle
	socket.socketClosing = closing
	socket.socketSend, socket.socketFlush = socket.socketFlush[:0], socket.socketSend
	socket.socketWriterWake <- struct{}{}
}

// netWriteLoop writes each handed off buffer. After a failed write it posts
// the error once and discards the rest. It closes the socket after the last
// buffer when the program closed the handle.
func netWriteLoop(socket *netSocket) {
	failed := false
	for range socket.socketWriterWake {
		flush := socket.socketFlush
		var err error
		switch {
		case failed:
		case socket.socketKind == netSocketKindUDP:
			err = netDatagramsWrite(socket.socketPacket, flush)
		default:
			_, err = socket.socketStream.Write(flush)
		}
		if err != nil && !failed {
			failed = true
			netArrivalSend(socket, netArrival{arrivalSocket: socket, arrivalTrigger: NetTriggerError, arrivalDetail: int32(netErrorOf(err))})
		}
		if socket.socketClosing {
			if socket.socketStream != nil {
				socket.socketStream.Close()
			}
			if socket.socketPacket != nil {
				socket.socketPacket.Close()
			}
			return
		}
		socket.socketWriterIdle <- struct{}{}
	}
}

// netDatagramsWrite sends the datagrams netDatagramQueue framed.
func netDatagramsWrite(packet *net.UDPConn, frames []byte) error {
	for len(frames) >= netDatagramHeaderSize {
		address := netip.AddrPortFrom(netip.AddrFrom16([16]byte(frames[:16])).Unmap(), uint16(frames[16])<<8|uint16(frames[17]))
		length := int(frames[18])<<8 | int(frames[19])
		payload := frames[netDatagramHeaderSize : netDatagramHeaderSize+length]
		var err error
		if address.Port() == 0 {
			_, err = packet.Write(payload)
		} else {
			_, err = packet.WriteToUDPAddrPort(payload, address)
		}
		if err != nil {
			return err
		}
		frames = frames[netDatagramHeaderSize+length:]
	}
	return nil
}
