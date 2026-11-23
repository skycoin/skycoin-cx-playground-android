package cxrun

import (
	"errors"
	"net"
	"os"
	"sync"
	"time"

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
// A UDP connection sends and receives one datagram per call. Connections a
// program leaves open are closed when it ends.
var (
	netMu    sync.Mutex
	netConns = map[int32]net.Conn{}
	netNext  int32
)

func netConn(id int32) (net.Conn, error) {
	netMu.Lock()
	defer netMu.Unlock()
	c, ok := netConns[id]
	if !ok {
		return nil, errors.New("no such connection")
	}
	return c, nil
}

func errText(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, os.ErrDeadlineExceeded):
		return "timeout"
	}
	return err.Error()
}

func opNetDial(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	network, addr := in[0].Get_str(p), in[1].Get_str(p)
	var c net.Conn
	err := errors.New("network must be tcp or udp")
	if network == "tcp" || network == "udp" {
		c, err = net.DialTimeout(network, addr, 10*time.Second)
	}
	id := int32(-1)
	if err == nil {
		netMu.Lock()
		netNext++
		id = netNext
		netConns[id] = c
		netMu.Unlock()
	}
	out[0].Set_i32(p, id)
	out[1].Set_str(p, errText(err))
}

func opNetSend(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	c, err := netConn(in[0].Get_i32(p))
	if err == nil {
		_, err = c.Write([]byte(in[1].Get_str(p)))
	}
	out[0].Set_str(p, errText(err))
}

func opNetRecv(p *cxast.CXProgram, in []cxast.CXValue, out []cxast.CXValue) {
	c, err := netConn(in[0].Get_i32(p))
	var n int
	buf := make([]byte, 64<<10)
	if err == nil {
		c.SetReadDeadline(time.Now().Add(time.Duration(in[1].Get_i32(p)) * time.Millisecond))
		n, err = c.Read(buf)
	}
	out[0].Set_str(p, string(buf[:n]))
	out[1].Set_str(p, errText(err))
}

func opNetClose(p *cxast.CXProgram, in []cxast.CXValue, _ []cxast.CXValue) {
	id := in[0].Get_i32(p)
	netMu.Lock()
	c := netConns[id]
	delete(netConns, id)
	netMu.Unlock()
	if c != nil {
		c.Close()
	}
}

// netCloseAll closes the connections of the program that ended.
func netCloseAll() {
	netMu.Lock()
	defer netMu.Unlock()
	for id, c := range netConns {
		c.Close()
		delete(netConns, id)
	}
}
