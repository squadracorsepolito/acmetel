package processor

import (
	"context"
	"fmt"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

//////////////
//  CONFIG  //
//////////////

type CustomConfig struct {
	PoolConfig *pool.Config

	Name string
}

func DefaultCustomConfig() *CustomConfig {
	return &CustomConfig{
		PoolConfig: pool.DefaultConfig(),

		Name: "custom",
	}
}

///////////////
//  HANDLER  //
///////////////

// CustomHandler interface defines the methods that the handler for the
// processor processor must implement.
type CustomHandler[In msg, T any, Out msgPtr[T]] interface {
	// Init method is called once when the stage is initialized.
	Init(ctx context.Context) error

	// Handle method is called by one of the spawned workers
	// for each message received by the stage.
	Handle(ctx context.Context, msgIn In, msgOut Out) error

	// Close is called once when the stage is closed.
	Close()
}

//////////////
//  WORKER  //
//////////////

type customWorkerArgs[In msg, T any, Out msgPtr[T]] struct {
	name    string
	handler CustomHandler[In, T, Out]
}

func newCustomWorkerArgs[In msg, T any, Out msgPtr[T]](name string, handler CustomHandler[In, T, Out]) *customWorkerArgs[In, T, Out] {
	return &customWorkerArgs[In, T, Out]{
		name:    name,
		handler: handler,
	}
}

type customWorker[In msg, T any, Out msgPtr[T]] struct {
	tel *internal.Telemetry

	handler CustomHandler[In, T, Out]

	traceString string
}

func (cw *customWorker[In, T, Out]) SetTelemetry(tel *internal.Telemetry) {
	cw.tel = tel
}

func (cw *customWorker[In, T, Out]) Init(_ context.Context, args *customWorkerArgs[In, T, Out]) error {
	cw.handler = args.handler

	cw.traceString = fmt.Sprintf("handle %s message", args.name)

	return nil
}

func (cw *customWorker[In, T, Out]) Handle(ctx context.Context, msgIn In) (Out, error) {
	// Extract the span context from the input message
	ctx, span := cw.tel.NewTrace(msgIn.LoadSpanContext(ctx), cw.traceString)
	defer span.End()

	// Create the generic output message
	var dummyMsgOut T
	msgOut := Out(&dummyMsgOut)

	// Call the provided handler
	if err := cw.handler.Handle(ctx, msgIn, msgOut); err != nil {
		return msgOut, err
	}

	// Save the span context into the output message
	msgOut.SaveSpan(span)

	return msgOut, nil
}

func (cw *customWorker[In, T, Out]) Close(_ context.Context) error {
	return nil
}

/////////////
//  STAGE  //
/////////////

type CustomStage[In msg, T any, Out msgPtr[T]] struct {
	*stage.Processor[In, Out, customWorker[In, T, Out], *customWorkerArgs[In, T, Out], *customWorker[In, T, Out]]

	cfg *CustomConfig

	handler CustomHandler[In, T, Out]
}

func NewCustomStage[In msg, T any, Out msgPtr[T]](
	handler CustomHandler[In, T, Out], inputConnector conn[In], outputConnector conn[Out], cfg *CustomConfig,
) *CustomStage[In, T, Out] {

	return &CustomStage[In, T, Out]{
		Processor: stage.NewProcessor[In, Out, customWorker[In, T, Out], *customWorkerArgs[In, T, Out]](
			cfg.Name, inputConnector, outputConnector, cfg.PoolConfig,
		),

		cfg: cfg,

		handler: handler,
	}
}

func (cs *CustomStage[In, T, Out]) Init(ctx context.Context) error {
	// Initialize the handler
	if err := cs.handler.Init(ctx); err != nil {
		return err
	}

	return cs.Processor.Init(ctx, newCustomWorkerArgs(cs.cfg.Name, cs.handler))
}
