package cannelloni

import (
	"time"

	"github.com/squadracorsepolito/acmetel/can"
	"github.com/squadracorsepolito/acmetel/internal"
)

var _ internal.ReOrderableMessage = (*Message)(nil)

const (
	// maximum number of CAN 2.0 messages (8 bytes payload) that can be sent in a single udp/ipv4/ethernet packet
	defaultCANMessageNum = 113
)

type RawCANMessage struct {
	CANID   uint32
	DataLen int
	RawData []byte
}

func (r *RawCANMessage) GetCANID() uint32 {
	return r.CANID
}

func (r *RawCANMessage) GetDataLength() int {
	return r.DataLen
}

func (r *RawCANMessage) GetRawData() []byte {
	return r.RawData
}

type Message struct {
	internal.BaseMessage

	SeqNum       uint8
	Timestamp    time.Time
	MessageCount int
	Messages     []can.RawMessage
}

func (msg *Message) GetSequenceNumber() uint64 {
	return uint64(msg.SeqNum)
}

func (msg *Message) LogicalTime() time.Time {
	return msg.Timestamp
}

func (msg *Message) SetAdjustedTime(adjustedTime time.Time) {
	msg.Timestamp = adjustedTime
}

func (msg *Message) GetRawCANMessages() []can.RawMessage {
	return msg.Messages[:msg.MessageCount]
}

func newMessage() *Message {
	return &Message{
		MessageCount: 0,
		Messages:     make([]can.RawMessage, defaultCANMessageNum),
	}
}
