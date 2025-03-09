package main

import (
	"fmt"
	"log"
	"net"
	"net/netip"
	"time"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/cannelloni"
)

const serverIP = "127.0.0.1"
const serverPort = 20_000
const bufferSize = 2048

type Message struct {
	Timestamp time.Time
	CANID     acmelib.CANID
	DataLen   int
	RawData   []byte
}

func newMessage(timestamp time.Time, cannelloniMsg *cannelloni.CannelloniFrameMessage) *Message {
	return &Message{
		Timestamp: timestamp,
		CANID:     acmelib.CANID(cannelloniMsg.CANID),
		DataLen:   int(cannelloniMsg.DataLen),
		RawData:   cannelloniMsg.Data,
	}
}

func (m *Message) String() string {
	timeStr := fmt.Sprintf("%s", m.Timestamp.Format(time.StampMilli))
	return fmt.Sprintf("%s -> CANID: %d, DataLen: %d, Data: %s", timeStr, m.CANID, m.DataLen, m.RawData)
}

func main() {
	addr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(serverIP), serverPort))

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	buf := make([]byte, bufferSize)

	for {
		n, err := conn.Read(buf)
		if err != nil {
			panic(err)
		}

		f, err := cannelloni.DecodeFrame(buf[:n])
		if err != nil {
			panic(err)
		}

		timestamp := time.Now()

		// log.Printf("Version: %d, OPCode: %d, SequenceNumber: %d, MessageCount: %d", f.Version, f.OPCode, f.SequenceNumber, f.MessageCount)

		for _, cannelloniMsg := range f.Messages {
			// log.Printf("%s -> CANID: %d, DataLen: %d, Data: %s", timestamp.Format(time.RFC3339), msg.CANID, msg.DataLen, msg.Data)
			msg := newMessage(timestamp, cannelloniMsg)
			log.Print(msg)
		}
	}
}
