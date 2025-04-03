package main

import (
	"log"
	"net"
	"net/netip"
	"time"

	"github.com/squadracorsepolito/acmetel/cannelloni"
)

func main() {
	addr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), 20_000))

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}

	f := cannelloni.NewFrame(0, 0)

	for i := range 128 {
		msg := cannelloni.NewFrameMessage(uint32(i*1000), []byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8})
		f.AddMessage(msg)
	}

	data := f.Encode()
	log.Print("packet size: ", len(data))

	t1 := time.Now()

	udpPackets := 1_000_000
	for range udpPackets {
		_, err = conn.Write(data)
		if err != nil {
			panic(err)
		}

		// if i%1000 == 0 {
		// 	time.Sleep(time.Millisecond * 10)
		// }

		// f.SequenceNumber++
	}

	t2 := time.Now()

	log.Print("packets per sec: ", float64(udpPackets)/t2.Sub(t1).Seconds())
}
