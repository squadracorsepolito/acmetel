package handler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
)

type stage[MIn, MOut message.Message, Cfg worker.ConfigurablePool, W, WArgs any, WPtr worker.HandlerWorkerPtr[W, WArgs, MIn, MOut]] struct {
	tel *internal.Telemetry

	cfg Cfg

	inputConnector  connector.Connector[MIn]
	outputConnector connector.Connector[MOut]

	writerWg *sync.WaitGroup

	workerPool *worker.Pool[W, WArgs, MIn, MOut, WPtr]

	skippedMessages atomic.Int64
}

func newStage[MIn, MOut message.Message, Cfg worker.ConfigurablePool, W, WArgs any, WPtr worker.HandlerWorkerPtr[W, WArgs, MIn, MOut]](name string, cfg Cfg) *stage[MIn, MOut, Cfg, W, WArgs, WPtr] {
	tel := internal.NewTelemetry("handler", name)

	return &stage[MIn, MOut, Cfg, W, WArgs, WPtr]{
		tel: tel,

		cfg: cfg,

		writerWg: &sync.WaitGroup{},

		workerPool: worker.NewPool[W, WArgs, MIn, MOut, WPtr](tel, cfg.ToPoolConfig()),
	}
}

func (s *stage[MIn, MOut, Cfg, W, WArgs, WPtr]) init(ctx context.Context, workerArgs WArgs) error {
	s.tel.LogInfo("initializing")
	defer s.tel.LogInfo("initialized")

	s.workerPool.Init(ctx, workerArgs)

	s.initMetrics()

	return nil
}

func (s *stage[MIn, MOut, Cfg, W, WArgs, WPtr]) initMetrics() {
	s.tel.NewCounter("skipped_messages", func() int64 { return s.skippedMessages.Load() })
}

func (s *stage[MIn, MOut, Cfg, W, WArgs, WPtr]) run(ctx context.Context) {
	s.tel.LogInfo("running")
	defer s.tel.LogInfo("stopped")

	go s.workerPool.Run(ctx)

	go s.runWriter(ctx)

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

func (s *stage[MIn, MOut, Cfg, W, WArgs, WPtr]) runWriter(ctx context.Context) {
	s.writerWg.Add(1)
	defer s.writerWg.Done()

	outputCh := s.workerPool.GetOutputCh()
	for {
		select {
		case <-ctx.Done():
			return
		case msgOut := <-outputCh:
			if err := s.outputConnector.Write(msgOut); err != nil {
				s.tel.LogError("failed to write into output connector", err)
			}
		}
	}
}

func (s *stage[MIn, MOut, Cfg, W, WArgs, WPtr]) close() {
	s.tel.LogInfo("closing")
	defer s.tel.LogInfo("closed")

	s.outputConnector.Close()
	s.workerPool.Stop()
	s.writerWg.Wait()
}

func (s *stage[MIn, MOut, Cfg, W, WArgs, WPtr]) SetInput(inputConnector connector.Connector[MIn]) {
	s.inputConnector = inputConnector
}

func (s *stage[MIn, MOut, Cfg, W, WArgs, WPtr]) SetOutput(outputConnector connector.Connector[MOut]) {
	s.outputConnector = outputConnector
}
