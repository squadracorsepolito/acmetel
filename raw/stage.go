package raw

import (
	"context"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

type msgOut[Out any] interface {
	*Out
	message.Message
}

type Stage[In message.Message, T any, Out msgOut[T]] struct {
	*stage.Handler[In, Out, worker[In, T, Out], *workerArgs[In, T, Out], *worker[In, T, Out]]

	name          string
	workerHandler Handler[In, T, Out]
}

func NewStage[In message.Message, T any, Out msgOut[T]](name string, handler Handler[In, T, Out], inputConnector connector.Connector[In], outputConnector connector.Connector[Out], cfg *Config) *Stage[In, T, Out] {
	if handler == nil {
		panic("handler is nil")
	}

	return &Stage[In, T, Out]{
		Handler: stage.NewHandler[In, Out, worker[In, T, Out], *workerArgs[In, T, Out]](
			name, inputConnector, outputConnector, cfg.PoolConfig,
		),

		name:          name,
		workerHandler: handler,
	}
}

func (s *Stage[In, T, Out]) Init(ctx context.Context) error {
	// Initialize the handler
	if err := s.workerHandler.Init(ctx); err != nil {
		return err
	}

	return s.Handler.Init(ctx, newWorkerArgs(s.name, s.workerHandler))
}
