package cannelloni

import (
	"github.com/squadracorsepolito/acmetel/can"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

var _ message.ReOrderable = (*Message)(nil)
var _ can.RawCANMessageCarrier = (*Message)(nil)

const (
	// maximum number of CAN 2.0 messages (8 bytes payload) that can be sent in a single udp/ipv4/ethernet packet
	defaultCANMessageNum = 113
)

// Message represents a cannelloni CAN message.
type Message struct {
	message.Base

	seqNum uint8

	// Messages is the list of CAN messages contained in a cannelloni frame.
	Messages []can.RawMessage
	// MessageCount is the number of CAN messages.
	MessageCount int
}

func newMessage() *Message {
	return &Message{
		MessageCount: 0,
		Messages:     make([]can.RawMessage, defaultCANMessageNum),
	}
}

// GetSequenceNumber returns the sequence number of the cannelloni frame.
func (msg *Message) GetSequenceNumber() uint64 {
	return uint64(msg.seqNum)
}

// GetRawMessages returns the list of CAN messages contained in the cannelloni frame.
func (msg *Message) GetRawMessages() []can.RawMessage {
	return msg.Messages[:msg.MessageCount]
}
