package message

import (
	"sync"
	"time"
)

const (
	// maximum number of CAN 2.0 messages (8 bytes payload) that can be sent in a single udp/ipv4/ethernet packet
	DefaultCANMessageNum = 113
)

type RawCANMessage struct {
	CANID   uint32
	DataLen int
	RawData []byte
}

var _ ReOrderableMessage = (*RawCANMessageBatch)(nil)

type RawCANMessageBatch struct {
	embedded

	SeqNum       uint8
	Timestamp    time.Time
	MessageCount int
	Messages     []RawCANMessage
}

func (msg *RawCANMessageBatch) SequenceNumber() uint64 {
	return uint64(msg.SeqNum)
}

func (msg *RawCANMessageBatch) LogicalTime() time.Time {
	return msg.Timestamp
}

func (msg *RawCANMessageBatch) SetLogicalTime(logicalTime time.Time) {
	msg.Timestamp = logicalTime
}

var rawCANMessageBatchPool = &sync.Pool{
	New: func() any {
		return &RawCANMessageBatch{
			MessageCount: 0,
			Messages:     make([]RawCANMessage, DefaultCANMessageNum),
		}
	},
}

func NewRawCANMessageBatch() *RawCANMessageBatch {
	return rawCANMessageBatchPool.Get().(*RawCANMessageBatch)
}

func PutRawCANMessageBatch(b *RawCANMessageBatch) {
	rawCANMessageBatchPool.Put(b)
}
