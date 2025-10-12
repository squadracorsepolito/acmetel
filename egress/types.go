// Package egress contains the egress stages.
package egress

import (
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

type msgSer = message.Serializable

type conn[T any] = connector.Connector[T]
