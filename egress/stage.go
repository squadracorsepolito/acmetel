package egress

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
)

type stage[M message.Message, Cfg worker.ConfigurablePool, W, WArgs any, WPtr worker.EgressWorkerPtr[W, WArgs, M]] struct {
	tel *internal.Telemetry

	cfg Cfg

	inputConnector connector.Connector[M]

	workerPool *worker.EgressPool[M, W, WArgs, WPtr]

	skippedMessages atomic.Int64
}

func newStage[M message.Message, Cfg worker.ConfigurablePool, W, WArgs any, WPtr worker.EgressWorkerPtr[W, WArgs, M]](name string, cfg Cfg) *stage[M, Cfg, W, WArgs, WPtr] {
	tel := internal.NewTelemetry("egress", name)

	return &stage[M, Cfg, W, WArgs, WPtr]{
		tel: tel,

		cfg: cfg,

		workerPool: worker.NewEgressPool[M, W, WArgs, WPtr](tel, cfg.ToPoolConfig()),
	}
}

func (s *stage[M, Cfg, W, WArgs, WPtr]) init(ctx context.Context, workerArgs WArgs) error {
	s.tel.LogInfo("initializing")
	defer s.tel.LogInfo("initialized")

	s.workerPool.Init(ctx, workerArgs)

	s.initMetrics()

	return nil
}

func (s *stage[M, Cfg, W, WArgs, WPtr]) initMetrics() {
	s.tel.NewCounter("skipped_messages", func() int64 { return s.skippedMessages.Load() })
}

func (s *stage[M, Cfg, W, WArgs, WPtr]) run(ctx context.Context) {
	s.tel.LogInfo("running")
	defer s.tel.LogInfo("stopped")

	go s.workerPool.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		msg, err := s.inputConnector.Read()
		if err != nil {
			if errors.Is(err, connector.ErrClosed) {
				s.tel.LogInfo("input connector is closed, stopping")
				return
			}

			s.tel.LogError("failed to read from input connector", err)
			continue
		}

		if !s.workerPool.AddTask(ctx, msg) {
			s.skippedMessages.Add(1)
		}
	}
}

func (s *stage[M, Cfg, W, WArgs, WPtr]) close() {
	s.tel.LogInfo("closing")
	defer s.tel.LogInfo("closed")

	s.workerPool.Stop()
}

func (s *stage[M, Cfg, W, WArgs, WPtr]) SetInput(inputConnector connector.Connector[M]) {
	s.inputConnector = inputConnector
}
