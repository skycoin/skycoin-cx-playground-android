// Package netecho echoes TCP streams and UDP datagrams on one port, the
// server of the network tests under tests/net-*.
package netecho

import (
	"io"
	"log"
	"net"
)

// Server echoes on the TCP and the UDP port of Addr.
type Server struct {
	Addr string
	l    net.Listener
	pc   net.PacketConn
}

// Listen starts echoing on addr. Port 0 picks one port free for both.
func Listen(addr string) (*Server, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenPacket("udp", l.Addr().String())
	if err != nil {
		l.Close()
		return nil, err
	}
	s := &Server{Addr: l.Addr().String(), l: l, pc: pc}
	go s.serveTCP()
	go s.serveUDP()
	return s, nil
}

// Close stops the server.
func (s *Server) Close() {
	s.l.Close()
	s.pc.Close()
}

func (s *Server) serveTCP() {
	for {
		c, err := s.l.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			n, err := io.Copy(c, c)
			log.Printf("tcp %s: echoed %d bytes, %v", c.RemoteAddr(), n, err)
		}()
	}
}

func (s *Server) serveUDP() {
	buf := make([]byte, 64<<10)
	for {
		n, from, err := s.pc.ReadFrom(buf)
		if err != nil {
			return
		}
		s.pc.WriteTo(buf[:n], from)
	}
}
