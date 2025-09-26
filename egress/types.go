// Package egress contains the egress stages.
package egress

import "github.com/squadracorsepolito/acmetel/connector"

type conn[T any] = connector.Connector[T]
