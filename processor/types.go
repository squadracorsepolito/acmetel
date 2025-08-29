package processor

import (
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

type msg = message.Message

type msgPtr[T any] interface {
	*T
	msg
}

type msgSer = message.Serializable

type conn[T any] = connector.Connector[T]
