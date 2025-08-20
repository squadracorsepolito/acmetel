package raw

import (
	"context"

	"github.com/squadracorsepolito/acmetel/internal/message"
)

type Handler[In, Out message.Message] interface {
	Init(context.Context) error
	Handle(context.Context, In) (Out, error)
	Close()
}
