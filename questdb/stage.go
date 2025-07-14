package questdb

import (
	"context"
	"time"

	qdb "github.com/questdb/go-questdb-client/v3"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

type Stage[T internal.Message] struct {
	*stage.Egress[T, worker[T], *workerArgs[T], *worker[T]]

	cfg *Config

	handler    Handler[T]
	senderPool *qdb.LineSenderPool
}

func NewStage[T internal.Message](handler Handler[T], inputConnector connector.Connector[T], cfg *Config) *Stage[T] {
	if handler == nil {
		panic("handler is nil")
	}

	return &Stage[T]{
		Egress: stage.NewEgress[T, worker[T], *workerArgs[T]]("questdb", inputConnector, cfg.PoolConfig),

		cfg: cfg,

		handler: handler,
	}
}

func (s *Stage[T]) Init(ctx context.Context) error {
	// Initialize the handler
	if err := s.handler.Init(ctx); err != nil {
		return err
	}

	// Create the sender pool
	senderPool, err := qdb.PoolFromOptions(
		qdb.WithAddress(s.cfg.Address),
		qdb.WithHttp(),
		qdb.WithAutoFlushRows(75_000),
		qdb.WithRetryTimeout(time.Second),
	)
	if err != nil {
		return err
	}
	s.senderPool = senderPool

	return s.Egress.Init(ctx, &workerArgs[T]{
		handler:    s.handler,
		senderPool: senderPool,
	})
}

func (s *Stage[T]) Stop() {
	s.Egress.Close()

	// Close the sender pool
	if err := s.senderPool.Close(context.Background()); err != nil {
		s.Tel.LogError("failed to close sender pool", err)
	}

	// Close the handler
	s.handler.Close()
}
