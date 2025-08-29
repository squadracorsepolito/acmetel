package pool

import (
	"context"

	"github.com/squadracorsepolito/acmetel/internal"
)

type worker[InitArgs any] interface {
	Init(ctx context.Context, args InitArgs) error
	Close(ctx context.Context) error
	SetTelemetry(tel *internal.Telemetry)
}

// ProcessorWorker is the interface for a processor worker.
type ProcessorWorker[InitArgs, In, Out any] interface {
	worker[InitArgs]

	Handle(ctx context.Context, task In) (Out, error)
}

// ProcessorWorkerPtr is an utility type for the processor worker.
type ProcessorWorkerPtr[W, InitArgs, In, Out any] interface {
	*W
	ProcessorWorker[InitArgs, In, Out]
}

// EgressWorker is the interface for an egress worker.
type EgressWorker[InitArgs, In any] interface {
	worker[InitArgs]

	Deliver(ctx context.Context, task In) error
}

// EgressWorkerPtr is an utility type for the egress worker.
type EgressWorkerPtr[W, InitArgs, In any] interface {
	*W
	EgressWorker[InitArgs, In]
}

// BaseWorker is the base struct for a worker that can be embedded.
type BaseWorker struct {
	Tel *internal.Telemetry
}

// SetTelemetry sets the telemetry for the worker.
func (w *BaseWorker) SetTelemetry(tel *internal.Telemetry) {
	w.Tel = tel
}
