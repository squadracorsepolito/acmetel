package stage

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/pool"
)

type Ingress[M msg, W, WArgs any, WPtr ingressWorkerPtr[W, WArgs, M]] struct {
	tel *internal.Telemetry

	outputConnector connector.Connector[M]

	writerWg *sync.WaitGroup

	workerPool *pool.Ingress[W, WArgs, M, WPtr]
}

func NewIngress[M msg, W, WArgs any, WPtr ingressWorkerPtr[W, WArgs, M]](
	name string, outputConnector connector.Connector[M], poolCfg *pool.Config,
) *Ingress[M, W, WArgs, WPtr] {

	tel := internal.NewTelemetry("ingress", name)

	return &Ingress[M, W, WArgs, WPtr]{
		tel: tel,

		outputConnector: outputConnector,

		writerWg: &sync.WaitGroup{},

		workerPool: pool.NewIngress[W, WArgs, M, WPtr](tel, poolCfg),
	}
}

func (i *Ingress[M, W, WArgs, WPtr]) Init(ctx context.Context, workerArgs WArgs) error {
	defer i.tel.LogInfo("initialized")

	i.workerPool.Init(ctx, workerArgs)

	return nil
}

func (i *Ingress[M, W, WArgs, WPtr]) runWriter(ctx context.Context) {
	i.writerWg.Add(1)
	defer i.writerWg.Done()

	inputCh := i.workerPool.GetOutputCh()

	for {
		select {
		case <-ctx.Done():
			return
		case msgOut := <-inputCh:
			if err := i.outputConnector.Write(msgOut); err != nil {
				i.tel.LogError("failed to write into output connector", err)
			}
		}
	}
}

func (i *Ingress[M, W, WArgs, WPtr]) Run(ctx context.Context) {
	i.tel.LogInfo("running")
	defer i.tel.LogInfo("stopped")

	go i.runWriter(ctx)

	i.workerPool.Run(ctx)
}

func (i *Ingress[M, W, WArgs, WPtr]) Close() {
	i.tel.LogInfo("closing")

	i.outputConnector.Close()
	i.workerPool.Stop()
	i.writerWg.Wait()
}
