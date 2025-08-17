package pool

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/internal"
)

// Ingress is a worker pool intended to be used by an ingress stage.
type Ingress[W, InitArgs any, Out internal.Message, WPtr IngressWorkerPtr[W, InitArgs, Out]] struct {
	*withOutput[Out]

	tel *internal.Telemetry

	cfg *Config

	initArgs InitArgs

	wg *sync.WaitGroup

	receivedMessages atomic.Int64
	receivingErrors  atomic.Int64
}

// NewIngress returns a new ingress worker pool.
func NewIngress[W, InitArgs any, Out internal.Message, WPtr IngressWorkerPtr[W, InitArgs, Out]](tel *internal.Telemetry, cfg *Config) *Ingress[W, InitArgs, Out, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8

	return &Ingress[W, InitArgs, Out, WPtr]{
		withOutput: newWithOutput[Out](channelSize),

		tel: tel,

		cfg: cfg,

		wg: &sync.WaitGroup{},
	}
}

// Init initialises the worker pool.
func (ip *Ingress[W, InitArgs, Out, WPtr]) Init(_ context.Context, initArgs InitArgs) error {
	ip.initMetrics()

	ip.initArgs = initArgs
	return nil
}

func (ip *Ingress[W, InitArgs, Out, WPtr]) initMetrics() {
	ip.tel.NewCounter("worker_pool_received_messages", func() int64 { return ip.receivedMessages.Load() })
	ip.tel.NewCounter("worker_pool_receiving_errors", func() int64 { return ip.receivingErrors.Load() })
}

// Run runs the worker pool.
func (ip *Ingress[W, InitArgs, Out, WPtr]) Run(ctx context.Context) {
	ip.runWorker(ctx)
}

func (ip *Ingress[W, InitArgs, Out, WPtr]) runWorker(ctx context.Context) {
	var dummyWorker W
	worker := WPtr(&dummyWorker)

	worker.SetTelemetry(ip.tel)

	if err := worker.Init(ctx, ip.initArgs); err != nil {
		ip.tel.LogError("failed to init worker", err)
		return
	}

	defer func() {
		if err := worker.Close(context.Background()); err != nil {
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

// Close closes the worker pool.
func (ip *Ingress[W, InitArgs, Out, WPtr]) Close() {
	ip.tel.LogInfo("closing worker pool")

	ip.wg.Wait()
	ip.closeOutput()
}
