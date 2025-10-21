// Package ingress contains the ingress stages.
package ingress

import (
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

type msg = message.Message

type conn[T any] = connector.Connector[T]
