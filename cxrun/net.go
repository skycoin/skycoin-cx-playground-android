package cxrun

import (
	netbatch "github.com/skycoin/skycoin-cx/t1-netbatch"
	cxast "github.com/skycoin/skycoin-cx/t3-ast"
)

// The network calls of the app package. A connection is a number from
// app.NetDial, and errors are text, "" for none, as CX has no error type.
//
//	app.NetDial(network, address str) (conn i32, err str)  network is tcp or udp
//	app.NetSend(conn i32, data str) (err str)
//	app.NetRecv(conn i32, timeoutMs i32) (data str, err str)  err "timeout" when nothing came
//	app.NetClose(conn i32)
//
// They run on the batched runtime of the CX net library, skycoin-cx
// t1-netbatch (skycoin-cx#508), as sys/net does. A call that waits makes net
// updates until an entry of its connection is in a batch, and keeps the
// entries of the other app connections for their own calls. The batches are
// the whole network input of a run, so Options.NetRecord and NetReplay record
// and replay them. A program uses app.Net* or sys/net, not both, since the
// updates of one take the entries of the other.
var appConns = map[int32]*appConn{}

// appConn is an app connection and the entries of its socket that no call
// has taken yet.
type appConn struct {
	udp bool
	// ended is the error of a closed or failed connection, which every
	// later call returns.
	ended   string
	entries []appEntry
}

type appEntry struct {
	trigger netbatch.NetTrigger
	detail  int32
	payload []byte
}

// appErrors are the texts of netbatch.NetError.
var appErrors = [netbatch.NetErrorCount]string{"", "error", "connection refused", "connection reset", "timeout", "bad address", "address in use", "socket table full"}

const errReplayEnded = "net replay record ended"

// appUpdate makes one net update and sorts its entries into the queues of
// the app connections. It reports false when a replay has no more updates.
func appUpdate(waitMilliseconds int32) bool {
	count := netbatch.NetUpdate(waitMilliseconds)
	if count < 0 {
		return false
	}
	for range count {
		handle, trigger, message, detail := netbatch.NetReadyNext()
		c := appConns[handle]
		if c == nil {
			continue
		}
		e := appEntry{trigger: trigger, detail: detail}
		if trigger == netbatch.NetTriggerData {
			e.payload = make([]byte, detail)
			netbatch.NetRead(handle, message, e.payload, 0)
		}
		c.entries = append(c.entries, e)
	}
	return true
}

// take removes and returns the first queued entry of c whose trigger is
// one of want.
func (c *appConn) take(want ...netbatch.NetTrigger) (appEntry, bool) {
	for i, e := range c.entries {
		for _, w := range want {
			if e.trigger == w {
				c.entries = append(c.entries[:i], c.entries[i+1:]...)
				if w == netbatch.NetTriggerClosed || w == netbatch.NetTriggerError {
					c.ended = errorText(e)
				}
				return e, true
			}
		}
	}
	return appEntry{}, false
}

func errorText(e appEntry) string {
	switch e.trigger {
	case netbatch.NetTriggerClosed:
		return "EOF"
	case netbatch.NetTriggerTimeout:
		return "timeout"
	case netbatch.NetTriggerError:
		if e.detail > 0 && e.detail < int32(len(appErrors)) {
			return appErrors[e.detail]
		}
		return "error"
	}
	return ""
}

func opNetDial(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	network, addr := in[0].Get_str(p), in[1].Get_str(p)
	id, err := netDial(network, addr)
	out[0].Set_i32(p, id)
	out[1].Set_str(p, err)
}

// netDial opens the socket and makes updates until it is connected or failed.
func netDial(network, addr string) (int32, string) {
	var handle int32
	switch network {
	case "tcp":
		handle = netbatch.NetTCPDial(addr)
	case "udp":
		handle = netbatch.NetUDPDial(addr)
	default:
		return -1, "network must be tcp or udp"
	}
	if handle == 0 {
		return -1, appErrors[netbatch.NetErrorSocketTableFull]
	}
	c := &appConn{udp: network == "udp"}
	appConns[handle] = c
	for {
		if _, ok := c.take(netbatch.NetTriggerConnected); ok {
			return handle, ""
		}
		if e, ok := c.take(netbatch.NetTriggerError); ok {
			netClose(handle)
			return -1, errorText(e)
		}
		if !appUpdate(1000) {
			netClose(handle)
			return -1, errReplayEnded
		}
	}
}

func opNetSend(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	out[0].Set_str(p, netSend(in[0].Get_i32(p), []byte(in[1].Get_str(p))))
}

// netSend queues data, making updates while the send buffer of an update is
// full. A UDP datagram is queued whole or not at all.
func netSend(handle int32, data []byte) string {
	c := appConns[handle]
	if c == nil {
		return "no such connection"
	}
	if c.ended != "" {
		return c.ended
	}
	for tries := 0; len(data) > 0; tries++ {
		if e, ok := c.take(netbatch.NetTriggerError, netbatch.NetTriggerClosed); ok {
			return errorText(e)
		}
		queued := netbatch.NetWrite(handle, data)
		if c.udp && queued == 0 && tries > 0 {
			// The update before emptied the send buffer, so it never fits.
			return "datagram too long"
		}
		data = data[queued:]
		if len(data) > 0 && !appUpdate(0) {
			return errReplayEnded
		}
	}
	return ""
}

func opNetRecv(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	data, err := netRecv(in[0].Get_i32(p), in[1].Get_i32(p))
	out[0].Set_str(p, string(data))
	out[1].Set_str(p, err)
}

// netRecv returns the next payload of handle. The timeout is a timeout of
// the net runtime, so it is an entry of a batch and replays as one.
func netRecv(handle int32, timeoutMilliseconds int32) ([]byte, string) {
	c := appConns[handle]
	if c == nil {
		return nil, "no such connection"
	}
	// A timeout of an earlier call that fired in the batch its data came in.
	for {
		if _, ok := c.take(netbatch.NetTriggerTimeout); !ok {
			break
		}
	}
	waits := timeoutMilliseconds > 0
	if waits {
		netbatch.NetTimeoutSet(handle, timeoutMilliseconds)
		defer netbatch.NetTimeoutSet(handle, 0)
	}
	for updated := false; ; updated = true {
		if e, ok := c.take(netbatch.NetTriggerData, netbatch.NetTriggerClosed, netbatch.NetTriggerError, netbatch.NetTriggerTimeout); ok {
			return e.payload, errorText(e)
		}
		if c.ended != "" {
			// Data that came before the end was returned first.
			return nil, c.ended
		}
		if updated && !waits {
			return nil, "timeout"
		}
		wait := int32(0)
		if waits {
			wait = timeoutMilliseconds
		}
		if !appUpdate(wait) {
			return nil, errReplayEnded
		}
	}
}

func opNetClose(p *cxast.CXProgram, in []cxast.CXValue, _ []cxast.CXValue) {
	netClose(in[0].Get_i32(p))
}

func netClose(handle int32) {
	if appConns[handle] == nil {
		return
	}
	delete(appConns, handle)
	netbatch.NetClose(handle)
}

// netRunStart starts the net runtime for a run, live or recording or
// replaying as opts say.
func netRunStart(opts Options) error {
	clear(appConns)
	mode, path := netbatch.NetModeLive, ""
	switch {
	case opts.NetRecord != "" && opts.NetReplay != "":
		return errNetRecordAndReplay
	case opts.NetRecord != "":
		mode, path = netbatch.NetModeRecord, opts.NetRecord
	case opts.NetReplay != "":
		mode, path = netbatch.NetModeReplay, opts.NetReplay
	}
	return netbatch.NetRunStart(mode, path)
}

// netRunStop closes the sockets the program left open and finishes a
// recording.
func netRunStop() {
	clear(appConns)
	netbatch.NetRunStop()
}
