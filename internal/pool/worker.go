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

type EgressWorker[InitArgs, In any] interface {
	worker[InitArgs]
	Deliver(ctx context.Context, task In) error
}

type EgressWorkerPtr[W, InitArgs, In any] interface {
	*W
	EgressWorker[InitArgs, In]
}

type HandlerWorker[InitArgs, In, Out any] interface {
	worker[InitArgs]
	Handle(ctx context.Context, task In) (Out, error)
}

type HandlerWorkerPtr[W, InitArgs, In, Out any] interface {
	*W
	HandlerWorker[InitArgs, In, Out]
}

type IngressWorker[InitArgs, Out any] interface {
	worker[InitArgs]
	Receive(ctx context.Context) (Out, bool, error)
}

type IngressWorkerPtr[W, InitArgs, Out any] interface {
	*W
	IngressWorker[InitArgs, Out]
}
