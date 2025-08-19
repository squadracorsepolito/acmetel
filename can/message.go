package can

import (
	"github.com/squadracorsepolito/acmetel/internal"
)

type RawMessage struct {
	CANID   uint32
	DataLen int
	RawData []byte
}

type ValueType int

const (
	ValueTypeFlag ValueType = iota
	ValueTypeInt
	ValueTypeFloat
	ValueTypeEnum
)

type CANSignalTable int

type CANSignal struct {
	CANID      int64
	Name       string
	RawValue   uint64
	Type       ValueType
	ValueFlag  bool
	ValueInt   int64
	ValueFloat float64
	ValueEnum  string
}

type Message struct {
	internal.BaseMessage

	SignalCount int
	Signals     []CANSignal
}

func newMessage() *Message {
	return &Message{
		SignalCount: 0,
		Signals:     []CANSignal{},
	}
}
