package main

import (
	"log"
	"math/rand/v2"
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

	udpPackets := 100_000

	packets := make([][]byte, 0, udpPackets)
	for i := range udpPackets {
		f := cannelloni.NewFrame(0, 0)

		for i := range 113 {
			val := uint8(rand.Int32N(255))
			msg := cannelloni.NewFrameMessage(uint32(i), []byte{val, val, val, val, val, val, val, val})
			f.AddMessage(msg)
		}

		f.SequenceNumber = uint8(i % 255)
		data := f.Encode()

		packets = append(packets, data)
	}

	packetSize := len(packets[0])
	log.Print("packet size: ", packetSize)

	t1 := time.Now()

	for i, data := range packets {
		_, err = conn.Write(data)
		if err != nil {
			panic(err)
		}

		if i%100 == 0 {
			time.Sleep(time.Millisecond * 100)
		}
	}

	t2 := time.Now()

	packetsPerSec := float64(udpPackets) / t2.Sub(t1).Seconds()
	log.Print("packets per sec: ", packetsPerSec)
	log.Print("bytes per sec: ", packetsPerSec*float64(packetSize))
}
