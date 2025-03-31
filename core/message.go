package core

import (
	"fmt"
	"time"

	"github.com/squadracorsepolito/acmelib"
)

type Message struct {
	Timestamp time.Time
	CANID     acmelib.CANID
	DataLen   int
	RawData   []byte
}

func NewMessage(timestamp time.Time, canID acmelib.CANID, dataLen int, rawData []byte) *Message {
	return &Message{
		Timestamp: timestamp,
		CANID:     canID,
		DataLen:   dataLen,
		RawData:   rawData,
	}
}

func (m *Message) String() string {
	timeStr := fmt.Sprintf("%s", m.Timestamp.Format(time.StampMilli))
	return fmt.Sprintf("%s -> CANID: %d, DataLen: %d, Data: %s", timeStr, m.CANID, m.DataLen, m.RawData)
}
