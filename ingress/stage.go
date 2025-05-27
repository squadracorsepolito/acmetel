package ingress

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
)

type stage[MOut message.Message, Cfg worker.ConfigurablePool, W, WArgs any, WPtr worker.IngressWorkerPtr[W, WArgs, MOut]] struct {
	tel *internal.Telemetry

	cfg Cfg

	outputConnector connector.Connector[MOut]

	writerWg *sync.WaitGroup

	workerPool *worker.IngressPool[W, WArgs, MOut, WPtr]
}

func newStage[MOut message.Message, Cfg worker.ConfigurablePool, W, WArgs any, WPtr worker.IngressWorkerPtr[W, WArgs, MOut]](name string, cfg Cfg) *stage[MOut, Cfg, W, WArgs, WPtr] {
	tel := internal.NewTelemetry("ingress", name)

	return &stage[MOut, Cfg, W, WArgs, WPtr]{
		tel: tel,

		cfg: cfg,

		writerWg: &sync.WaitGroup{},

		workerPool: worker.NewIngressPool[W, WArgs, MOut, WPtr](tel, cfg.ToPoolConfig()),
	}
}

func (s *stage[MOut, Cfg, W, WArgs, WPtr]) init(ctx context.Context, workerArgs WArgs) error {
	s.tel.LogInfo("initializing")
	defer s.tel.LogInfo("initialized")

	s.workerPool.Init(ctx, workerArgs)

	return nil
}

func (s *stage[MOut, Cfg, W, WArgs, WPtr]) run(ctx context.Context) {
	s.tel.LogInfo("running")
	defer s.tel.LogInfo("stopped")

	go s.runWriter(ctx)

	s.workerPool.Run(ctx)
}

func (s *stage[MOut, Cfg, W, WArgs, WPtr]) runWriter(ctx context.Context) {
	s.writerWg.Add(1)
	defer s.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case msgOut := <-s.workerPool.GetOutputCh():
			if err := s.outputConnector.Write(msgOut); err != nil {
				s.tel.LogError("failed to write into output connector", err)
			}
		}
	}
}

func (s *stage[MOut, Cfg, W, WArgs, WPtr]) close() {
	s.tel.LogInfo("closing")
	defer s.tel.LogInfo("closed")

	s.outputConnector.Close()
	s.workerPool.Stop()
	s.writerWg.Wait()
}

func (s *stage[MOut, Cfg, W, WArgs, WPtr]) SetOutput(outputConnector connector.Connector[MOut]) {
	s.outputConnector = outputConnector
}
