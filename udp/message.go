package udp

import (
	"github.com/squadracorsepolito/acmetel/internal/message"
)

var _ message.Serializable = (*Message)(nil)

// Message represents a UDP message.
type Message struct {
	message.Base

	// Payload of the UDP datagram.
	Payload []byte
	// PayloadSize is the number of bytes of the payload.
	PayloadSize int
}

func newMessage(payload []byte, payloadSize int) *Message {
	return &Message{
		Payload:     payload,
		PayloadSize: payloadSize,
	}
}

// GetBytes returns the bytes of the UDP payload.
func (m *Message) GetBytes() []byte {
	return m.Payload
}
