package cannelloni

import (
	"encoding/binary"
	"testing"

	"github.com/squadracorsepolito/acmetel/internal"
)

type dummyMsgIn struct {
	internal.BaseMessage
}

func (d *dummyMsgIn) GetRawData() []byte {
	return nil
}

func Benchmark_cannelloniWorker_decodeFrame(b *testing.B) {
	b.ReportAllocs()

	adapter := &worker[*dummyMsgIn]{}
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
