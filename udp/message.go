package udp

import (
	"github.com/squadracorsepolito/acmetel/internal/message"
)

var _ message.Serializable = (*Message)(nil)

// Message represents a UDP message.
type Message struct {
	message.Base

	// Data is the payload of a UDP datagram.
	Data []byte
	// DataLen is the number of bytes of the payload.
	DataLen int
}

func newMessage(data []byte, dataLen int) *Message {
	return &Message{
		Data:    data,
		DataLen: dataLen,
	}
}

// GetBytes returns the bytes of the UDP payload.
func (m *Message) GetBytes() []byte {
	return m.Data
}
