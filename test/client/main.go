package main

import (
	"log"
	"math/rand/v2"
	"net"
	"net/netip"
	"time"

	"github.com/squadracorsepolito/acmetel/test/client/cannelloni"
)

const (
	cycles            = 3
	udpPackets        = 100_000
	messagesPerPacket = 50 // max 113
)

func main() {
	addr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), 20000))

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		panic(err)
	}

	packets := make([][]byte, 0, udpPackets)
	for i := range udpPackets {
		f := cannelloni.NewFrame(uint8(i%256), 0)

		for range messagesPerPacket {
			intVal := uint8(rand.Int32N(255))
			enumVal := uint8(rand.Int32N(3))

			msg := cannelloni.NewFrameMessage(1, []byte{intVal, intVal, intVal, intVal, intVal, intVal, intVal, enumVal})
			f.AddMessage(msg)
		}

		data := f.Encode()

		packets = append(packets, data)
	}

	packetSize := len(packets[0])
	log.Printf("cycles: %d; packets: %d; messages per packet: %d; packet size: %d",
		cycles, udpPackets, messagesPerPacket, packetSize)

	for c := range cycles {
		t1 := time.Now()

		for i, data := range packets {
			_, err = conn.Write(data)
			if err != nil {
				panic(err)
			}

			if i%10 == 0 {
				time.Sleep(time.Millisecond * 10)
			}
		}

		t2 := time.Now()

		packetsPerSec := float64(udpPackets) / t2.Sub(t1).Seconds()

		log.Printf("cycle %d: packets per sec: %f", c, packetsPerSec)
		log.Printf("cycle %d: bytes per sec: %f", c, packetsPerSec*float64(packetSize))

		time.Sleep(time.Second)
	}
}
