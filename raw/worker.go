package raw

import (
	"context"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

type workerArgs[In, Out message.Message] struct {
	handler Handler[In, Out]
}

func newWorkerArgs[In, Out message.Message](handler Handler[In, Out]) *workerArgs[In, Out] {
	return &workerArgs[In, Out]{handler: handler}
}

type worker[In, Out message.Message] struct {
	tel *internal.Telemetry

	handler Handler[In, Out]
}

func (w *worker[In, Out]) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *worker[In, Out]) Init(_ context.Context, args *workerArgs[In, Out]) error {
	w.handler = args.handler
	return nil
}

func (w *worker[In, Out]) Handle(ctx context.Context, msg In) (Out, error) {
	ctx, span := w.tel.NewTrace(ctx, "process raw message")
	defer span.End()

	return w.handler.Handle(ctx, msg)
}

func (w *worker[In, Out]) Close(_ context.Context) error {
	return nil
}
