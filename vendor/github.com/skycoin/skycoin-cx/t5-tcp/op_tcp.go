package tcp

import (
	"context"
	"net"
	"net/rpc"
	"time"

	"github.com/skycoin/skycoin-cx/t3-ast"
)

// todo need to find a way to support Listener and connection interface
// VIOLATION: state outside State. Owner: t5-tcp; moves to TCPListenerMng in phase 1.
var ln net.Listener

var lc net.ListenConfig

// VIOLATION: state outside State. Owner: t5-tcp; moves to TCPConnMng in phase 1.
var conn net.Conn

// VIOLATION: init chaining. Init 3: the tcp package RPC server, called from Go
// package init.
// VIOLATION: state outside State. Owner: t5-tcp; moves to RPCServerMng in phase 1.
var DefaultServer = rpc.NewServer()

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func opTCPDial(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	c, err := net.Dial(inputs[0].Get_str(prgrm), inputs[1].Get_str(prgrm))
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		conn = c
	}
	outputs[0].Set_str(prgrm, errString(err))
}

func opTCPClose(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	if ln != nil {
		ln.Close()
		ln = nil
	}
	if conn != nil {
		conn.Close()
		conn = nil
	}
}

func opTCPAccept(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	if ln == nil {
		outputs[0].Set_str(prgrm, "tcp: not listening")
		return
	}
	c, err := ln.Accept()
	if err == nil {
		if conn != nil {
			conn.Close()
		}
		conn = c
	}
	outputs[0].Set_str(prgrm, errString(err))
}

func opTCPListen(prgrm *ast.CXProgram, inputs []ast.CXValue, outputs []ast.CXValue) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	l, err := lc.Listen(ctx, inputs[0].Get_str(prgrm), inputs[1].Get_str(prgrm))
	if err == nil {
		if ln != nil {
			ln.Close()
		}
		ln = l
	}
	outputs[0].Set_str(prgrm, errString(err))
}
