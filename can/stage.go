package can

import (
	"context"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

type Stage[T RawCANMessageCarrier] struct {
	*stage.Handler[T, *Message, worker[T], *workerArgs, *worker[T]]

	messages []*acmelib.Message
}

func NewStage[T RawCANMessageCarrier](inputConnector connector.Connector[T], outputConnector connector.Connector[*Message], cfg *Config) *Stage[T] {
	return &Stage[T]{
		Handler: stage.NewHandler[T, *Message, worker[T], *workerArgs](
			"can", inputConnector, outputConnector, cfg.PoolConfig,
		),

		messages: cfg.Messages,
	}
}

func (s *Stage[T]) Init(ctx context.Context) error {
	decoder := newDecoder(s.messages)

	return s.Handler.Init(ctx, &workerArgs{decoder: decoder})
}
