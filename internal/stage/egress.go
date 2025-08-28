package stage

import (
	"context"
	"errors"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/rb"
)

type Egress[M msg, W, WA any, WP egressWorkerPtr[W, WA, M]] struct {
	Tel *internal.Telemetry

	inputConnector connector.Connector[M]

	workerPool *pool.Egress[M, W, WA, WP]
}

func NewEgress[M msg, W, WA any, WP egressWorkerPtr[W, WA, M]](name string, inputConnector connector.Connector[M], poolCfg *pool.Config) *Egress[M, W, WA, WP] {
	tel := internal.NewTelemetry("egress", name)

	return &Egress[M, W, WA, WP]{
		Tel: tel,

		inputConnector: inputConnector,

		workerPool: pool.NewEgress[M, W, WA, WP](tel, poolCfg),
	}
}

func (e *Egress[M, W, WA, WP]) Init(ctx context.Context, workerArgs WA) error {
	defer e.Tel.LogInfo("initialized")

	e.workerPool.Init(ctx, workerArgs)

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

			if !errors.Is(err, rb.ErrReadTimeout) {
				e.Tel.LogError("failed to read from input connector", err)
			}

			continue
		}

		e.workerPool.AddMessage(ctx, msg)
	}
}

func (e *Egress[M, W, WA, WP]) Close() {
	e.Tel.LogInfo("closing")

	e.workerPool.Close()
}
