package raw

import (
	"context"

	"github.com/squadracorsepolito/acmetel/internal"
)

type Handler[In, Out internal.Message] interface {
	Init(context.Context) error
	Handle(context.Context, In) (Out, error)
	Close()
}
