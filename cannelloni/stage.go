package cannelloni

import (
	"context"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

type Stage[T message.Serializable] struct {
	*stage.HandlerWithROB[T, *Message, worker[T], any, *worker[T]]
}

func NewStage[T message.Serializable](inputConnector connector.Connector[T], outputConnector connector.Connector[*Message], cfg *Config) *Stage[T] {
	return &Stage[T]{
		HandlerWithROB: stage.NewHandlerWithROB[T, *Message, worker[T], any](
			"cannelloni", inputConnector, outputConnector, cfg.PoolConfig, cfg.ROBConfig, cfg.ROBTimeout,
		),
	}
}

func (s *Stage[T]) Init(ctx context.Context) error {
	return s.HandlerWithROB.Init(ctx, nil)
}
