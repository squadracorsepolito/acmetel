package raw

import (
	"context"
	"fmt"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

type workerArgs[In message.Message, T any, Out msgOut[T]] struct {
	name    string
	handler Handler[In, T, Out]
}

func newWorkerArgs[In message.Message, T any, Out msgOut[T]](name string, handler Handler[In, T, Out]) *workerArgs[In, T, Out] {
	return &workerArgs[In, T, Out]{
		name:    name,
		handler: handler,
	}
}

type worker[In message.Message, T any, Out msgOut[T]] struct {
	tel *internal.Telemetry

	handler Handler[In, T, Out]

	traceString string
}

func (w *worker[In, T, Out]) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *worker[In, T, Out]) Init(_ context.Context, args *workerArgs[In, T, Out]) error {
	w.handler = args.handler

	w.traceString = fmt.Sprintf("handle %s message", args.name)

	return nil
}

func (w *worker[In, T, Out]) Handle(ctx context.Context, msgIn In) (Out, error) {
	// Extract the span context from the input message
	ctx, span := w.tel.NewTrace(msgIn.LoadSpanContext(ctx), w.traceString)
	defer span.End()

	// Create the generic output message
	var dummyMsgOut T
	msgOut := Out(&dummyMsgOut)

	// Set the receive time and timestamp
	msgOut.SetReceiveTime(msgIn.GetReceiveTime())
	msgOut.SetTimestamp(msgIn.GetTimestamp())

	// Call the provided handler
	if err := w.handler.Handle(ctx, msgIn, msgOut); err != nil {
		return msgOut, err
	}

	// Save the span context into the output message
	msgOut.SaveSpan(span)

	return msgOut, nil
}

func (w *worker[In, T, Out]) Close(_ context.Context) error {
	return nil
}
