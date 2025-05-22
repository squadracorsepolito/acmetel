package message

import (
	"sync"
	"time"
)

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

type CANSignalBatch struct {
	embedded

	Timestamp   time.Time
	SignalCount int
	Signals     []CANSignal
}

var canSignalBatchPool = &sync.Pool{
	New: func() any {
		return &CANSignalBatch{
			SignalCount: 0,
			Signals:     make([]CANSignal, 0, defaultCANSignalCount),
		}
	},
}

func NewCANSignalBatch() *CANSignalBatch {
	return &CANSignalBatch{
		SignalCount: 0,
		Signals:     []CANSignal{},
	}
	// return canSignalBatchPool.Get().(*CANSignalBatch)
}

func PutCANSignalBatch(sigBatch *CANSignalBatch) {
	// canSignalBatchPool.Put(sigBatch)
}
