package can

import (
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
)

type RawMessage struct {
	CANID   uint32
	DataLen int
	RawData []byte
}

const (
	defaultCANSignalCount = 904
)

type CANSignalTable int

const (
	CANSignalTableFlag CANSignalTable = iota
	CANSignalTableInt
	CANSignalTableFloat
	CANSignalTableEnum
)

func (c CANSignalTable) String() string {
	switch c {
	case CANSignalTableFlag:
		return "flag_signals"
	case CANSignalTableInt:
		return "int_signals"
	case CANSignalTableFloat:
		return "float_signals"
	case CANSignalTableEnum:
		return "enum_signals"
	default:
		return "unknown"
	}
}

type CANSignal struct {
	CANID      int64
	Name       string
	RawValue   int64
	Table      CANSignalTable
	ValueFlag  bool
	ValueInt   int64
	ValueFloat float64
	ValueEnum  string
}

type Message struct {
	internal.BaseMessage

	Timestamp   time.Time
	SignalCount int
	Signals     []CANSignal
}

func newMessage() *Message {
	return &Message{
		SignalCount: 0,
		Signals:     []CANSignal{},
	}
}
