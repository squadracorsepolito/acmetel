package raw

import (
	"context"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

type Stage[In, Out internal.Message] struct {
	*stage.Handler[In, Out, worker[In, Out], *workerArgs[In, Out], *worker[In, Out]]

	workerHandler Handler[In, Out]
}

func NewStage[In, Out internal.Message](handler Handler[In, Out], inputConnector connector.Connector[In], outputConnector connector.Connector[Out], cfg *Config) *Stage[In, Out] {
	if handler == nil {
		panic("handler is nil")
	}

	return &Stage[In, Out]{
		Handler: stage.NewHandler[In, Out, worker[In, Out], *workerArgs[In, Out]](
			"raw", inputConnector, outputConnector, cfg.PoolConfig,
		),

		workerHandler: handler,
	}
}

func (s *Stage[In, Out]) Init(ctx context.Context) error {
	// Initialize the handler
	if err := s.workerHandler.Init(ctx); err != nil {
		return err
	}

	return s.Handler.Init(ctx, newWorkerArgs(s.workerHandler))
}

func (s *Stage[In, Out]) Close() {
	s.Handler.Close()
}
