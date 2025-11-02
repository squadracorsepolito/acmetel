// Package ingress contains the ingress stages.
package ingress

import (
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

type msgEnv = message.Envelope

type msg[T msgEnv] = message.Message[T]

type msgSer = message.Serializable

type msgConn[T msgEnv] = connector.Connector[*msg[T]]
