package worker

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/internal"
)

type IngressPool[W, InitArgs any, Out internal.Message, WPtr IngressWorkerPtr[W, InitArgs, Out]] struct {
	*withOutput[Out]

	tel *internal.Telemetry

	cfg *PoolConfig

	initArgs InitArgs

	wg *sync.WaitGroup

	receivedMessages atomic.Int64
	receivingErrors  atomic.Int64
}

func NewIngressPool[W, InitArgs any, Out internal.Message, WPtr IngressWorkerPtr[W, InitArgs, Out]](tel *internal.Telemetry, cfg *PoolConfig) *IngressPool[W, InitArgs, Out, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8

	return &IngressPool[W, InitArgs, Out, WPtr]{
		withOutput: newWithOutput[Out](channelSize),

		tel: tel,

		cfg: cfg,

		wg: &sync.WaitGroup{},
	}
}

func (ip *IngressPool[W, InitArgs, Out, WPtr]) Init(_ context.Context, initArgs InitArgs) error {
	ip.initMetrics()

	ip.initArgs = initArgs
	return nil
}

func (ip *IngressPool[W, InitArgs, Out, WPtr]) initMetrics() {
	ip.tel.NewCounter("worker_pool_received_messages", func() int64 { return ip.receivedMessages.Load() })
	ip.tel.NewCounter("worker_pool_receiving_errors", func() int64 { return ip.receivingErrors.Load() })
}

func (ip *IngressPool[W, InitArgs, Out, WPtr]) Run(ctx context.Context) {
	ip.runWorker(ctx)
}

func (ip *IngressPool[W, InitArgs, Out, WPtr]) runWorker(ctx context.Context) {
	var dummyWorker W
	worker := WPtr(&dummyWorker)

	worker.SetTelemetry(ip.tel)

	if err := worker.Init(ctx, ip.initArgs); err != nil {
		ip.tel.LogError("failed to init worker", err)
		return
	}

	defer func() {
		if err := worker.Stop(context.Background()); err != nil {
			ip.tel.LogError("failed to stop worker", err)
		}
	}()

	ip.wg.Add(1)
	defer ip.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		tracedCtx, span := ip.tel.NewTrace(ctx, "receive message")

		msgOut, stop, err := worker.Receive(tracedCtx)
		if err != nil {
			ip.tel.LogError("failed to receive message", err)
			ip.receivingErrors.Add(1)

			goto loopCleanup
		}

		ip.receivedMessages.Add(1)

		ip.sendOutput(msgOut)

	loopCleanup:
		span.End()

		if stop {
			return
		}
	}
}

func (ip *IngressPool[W, InitArgs, Out, WPtr]) Stop() {
	ip.wg.Wait()
	ip.closeOutput()
}
