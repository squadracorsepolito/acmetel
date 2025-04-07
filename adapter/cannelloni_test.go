package adapter

import (
	"encoding/binary"
	"testing"
)

// 301087              3968 ns/op            5032 B/op        115 allocs/op
func Benchmark_Cannelloni_decodeFrame(b *testing.B) {
	b.ReportAllocs()

	adapter := NewCannelloni(DefaultCannelloniConfig())
	frame := getCannelloniEncodedFrame()

	b.ResetTimer()
	for b.Loop() {
		_, err := adapter.decodeFrame(frame)
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
