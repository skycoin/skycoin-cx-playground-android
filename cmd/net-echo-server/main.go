// Command net-echo-server echoes TCP streams and UDP datagrams on one port.
// The emulator driver runs it inside the rig container, which the emulator
// reaches as 10.0.2.2, for the network tests under tests/net-*.
package main

import (
	"flag"
	"log"

	"github.com/skycoin/skycoin-cx-playground-android/netecho"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:7001", "TCP and UDP address to echo on")
	flag.Parse()
	s, err := netecho.Listen(*addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("echoing TCP and UDP on %s", s.Addr)
	select {}
}
