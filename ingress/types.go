// Package ingress contains the ingress stages.
package ingress

import "github.com/squadracorsepolito/acmetel/connector"

type conn[T any] = connector.Connector[T]
