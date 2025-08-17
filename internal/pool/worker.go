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

// HandlerWorker is the interface for a handler worker.
type HandlerWorker[InitArgs, In, Out any] interface {
	worker[InitArgs]
	Handle(ctx context.Context, task In) (Out, error)
}

// HandlerWorkerPtr is an utility type for the handler worker.
type HandlerWorkerPtr[W, InitArgs, In, Out any] interface {
	*W
	HandlerWorker[InitArgs, In, Out]
}

// IngressWorker is the interface for an ingress worker.
type IngressWorker[InitArgs, Out any] interface {
	worker[InitArgs]
	Receive(ctx context.Context) (Out, bool, error)
}

// IngressWorkerPtr is an utility type for the ingress worker.
type IngressWorkerPtr[W, InitArgs, Out any] interface {
	*W
	IngressWorker[InitArgs, Out]
}
