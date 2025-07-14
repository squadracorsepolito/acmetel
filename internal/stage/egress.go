package stage

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/worker"
)

type Egress[M msg, W, WA any, WP egressWorkerPtr[W, WA, M]] struct {
	Tel *internal.Telemetry

	inputConnector connector.Connector[M]

	workerPool *worker.EgressPool[M, W, WA, WP]

	// Telemetry metrics
	skippedMessages atomic.Int64
}

func NewEgress[M msg, W, WA any, WP egressWorkerPtr[W, WA, M]](name string, inputConnector connector.Connector[M], poolCfg *worker.PoolConfig) *Egress[M, W, WA, WP] {
	tel := internal.NewTelemetry("egress", name)

	return &Egress[M, W, WA, WP]{
		Tel: tel,

		inputConnector: inputConnector,

		workerPool: worker.NewEgressPool[M, W, WA, WP](tel, poolCfg),
	}
}

func (e *Egress[M, W, WA, WP]) initMetrics() {
	e.Tel.NewCounter("skipped_messages", func() int64 { return e.skippedMessages.Load() })
}

func (e *Egress[M, W, WA, WP]) Init(ctx context.Context, workerArgs WA) error {
	defer e.Tel.LogInfo("initialized")

	e.workerPool.Init(ctx, workerArgs)

	e.initMetrics()

	return nil
}

func (e *Egress[M, W, WA, WP]) Run(ctx context.Context) {
	e.Tel.LogInfo("running")
	defer e.Tel.LogInfo("stopped")

	// Run the worker pool
	go e.workerPool.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		msg, err := e.inputConnector.Read()
		if err != nil {
			// Check if the input connector is closed, if so stop
			if errors.Is(err, connector.ErrClosed) {
				e.Tel.LogInfo("input connector is closed, stopping")
				return
			}

			e.Tel.LogError("failed to read from input connector", err)
			continue
		}

		// Push a new task to the worker pool
		if !e.workerPool.AddTask(ctx, msg) {
			e.skippedMessages.Add(1)
		}
	}
}

func (e *Egress[M, W, WA, WP]) Close() {
	e.Tel.LogInfo("closing")

	e.workerPool.Stop()
}
