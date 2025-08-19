package questdb

import (
	"context"
	"time"

	qdb "github.com/questdb/go-questdb-client/v3"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

type Stage struct {
	*stage.Egress[*Message, worker, *workerArgs, *worker]

	cfg *Config

	senderPool *qdb.LineSenderPool
}

func NewStage(inputConnector connector.Connector[*Message], cfg *Config) *Stage {
	return &Stage{
		Egress: stage.NewEgress[*Message, worker, *workerArgs]("questdb", inputConnector, cfg.PoolConfig),

		cfg: cfg,
	}
}

func (s *Stage) Init(ctx context.Context) error {
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

	return s.Egress.Init(ctx, newWorkerArgs(senderPool))
}

func (s *Stage) Close() {
	s.Egress.Close()

	// Close the sender pool
	if err := s.senderPool.Close(context.Background()); err != nil {
		s.Tel.LogError("failed to close sender pool", err)
	}
}
