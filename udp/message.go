package udp

import "github.com/squadracorsepolito/acmetel/internal"

var _ internal.RawDataMessage = (*Message)(nil)

type Message struct {
	internal.BaseMessage

	Data    []byte
	DataLen int
}

func (m *Message) GetRawData() []byte {
	return m.Data
}

func newMessage(data []byte, dataLen int) *Message {
	return &Message{
		Data:    data,
		DataLen: dataLen,
	}
}
