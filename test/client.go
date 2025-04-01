package main

import (
	"net"
	"net/netip"

	"github.com/squadracorsepolito/acmetel/cannelloni"
)

func main() {
	addr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), 20_000))

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}

	f := cannelloni.NewFrame(0, 0)

	for i := range 10 {
		msg := cannelloni.NewFrameMessage(uint32(i*1000), []byte("hello"))
		f.AddMessage(msg)
	}

	for range 10000000 {
		_, err = conn.Write(f.Encode())
		if err != nil {
			panic(err)
		}
		// time.Sleep(time.Millisecond * 10)

		f.SequenceNumber++
	}
}
