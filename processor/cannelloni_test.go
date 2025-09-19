package processor

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_cannelloniEncoder(t *testing.T) {
	assert := assert.New(t)

	encoder := newCannelloniEncoder()

	msgData := []byte{
		0b11000001,
		0b11000001,
	}

	msg := &cannelloniFrame{
		version:        1,
		opCode:         1,
		sequenceNumber: 128,
		messageCount:   2,
		messages: []cannelloniFrameMessage{
			{canID: 1, canFDFlags: 0, dataLen: 2, data: msgData},
			{canID: 0x0100, canFDFlags: 1, dataLen: 2, data: msgData},
		},
	}

	expected := []byte{
		// Header
		0x01, 0x01, 0x80,
		0, 0x02,

		// First message
		0, 0, 0, 0x01, // can-id
		0x02, // data len without can-fd flags
		0b11000001,
		0b11000001,

		// Second message
		0, 0, 0x01, 0, // can-id
		0x82, 1, // data len with can-fd flags (add | 0x80)
		0b11000001,
		0b11000001,
	}

	res := encoder.encode(msg)
	assert.Equal(expected, res)
}

func Benchmark_cannelloniDecoder(b *testing.B) {
	b.ReportAllocs()

	decoder := newCannelloniDecoder()
	frame := getCannelloniEncodedFrame()

	for b.Loop() {
		_, err := decoder.decode(frame)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func getCannelloniEncodedFrame() []byte {
	buf := make([]byte, 5)

	msgNum := 113

	buf[0] = 1
	buf[1] = 1
	buf[2] = 1
	binary.BigEndian.PutUint16(buf[3:5], uint16(msgNum))

	for canID := range msgNum {
		msgBuf := make([]byte, 13)

		binary.BigEndian.PutUint32(msgBuf[0:4], uint32(canID))
		msgBuf[4] = 8

		data := []byte{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8}

		for idx, tmpData := range data {
			msgBuf[5+idx] = tmpData
		}

		buf = append(buf, msgBuf...)
	}

	return buf
}
