package ticker

import "github.com/squadracorsepolito/acmetel/internal/message"

type Message struct {
	message.Base

	TriggerNumber int
}

func newMessage() *Message {
	return &Message{}
}
