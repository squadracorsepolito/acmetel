package processor

import (
	"context"
	"fmt"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/pool"
	stageCommon "github.com/squadracorsepolito/acmetel/internal/stage"
)

//////////////
//  CONFIG  //
//////////////

// CustomConfig structs contains the configuration for a custom processor stage.
type CustomConfig struct {
	Stage *stageCommon.Config

	// Name is the name of the stage.
	// It is used to identify the stage in the telemetry.
	//
	// Default: "custom"
	Name string
}

// DefaultCustomConfig returns the default configuration for a custom processor stage.
func DefaultCustomConfig(runningMode stageCommon.RunningMode) *CustomConfig {
	return &CustomConfig{
		Stage: stageCommon.DefaultConfig(runningMode),

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
	pool.BaseWorker

	handler CustomHandler[In, T, Out]

	traceString string
}

func newCustomWorkerInstMaker[In msg, T any, Out msgPtr[T]]() workerInstanceMaker[*customWorkerArgs[In, T, Out], In, Out] {
	return func() workerInstance[*customWorkerArgs[In, T, Out], In, Out] {
		return &customWorker[In, T, Out]{}
	}
}

func (cw *customWorker[In, T, Out]) SetTelemetry(tel *internal.Telemetry) {
	cw.Tel = tel
}

func (cw *customWorker[In, T, Out]) Init(_ context.Context, args *customWorkerArgs[In, T, Out]) error {
	cw.handler = args.handler

	cw.traceString = fmt.Sprintf("handle %s message", args.name)

	return nil
}

func (cw *customWorker[In, T, Out]) Handle(ctx context.Context, msgIn In) (Out, error) {
	// Extract the span context from the input message
	ctx, span := cw.Tel.NewTrace(msgIn.LoadSpanContext(ctx), cw.traceString)
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

// CustomStage is a processor stage that uses a custom handler to process messages.
type CustomStage[In msg, T any, Out msgPtr[T]] struct {
	stage[*customWorkerArgs[In, T, Out], In, Out]

	cfg *CustomConfig

	handler CustomHandler[In, T, Out]
}

// NewCustomStage returns a new custom processor stage.
func NewCustomStage[In msg, T any, Out msgPtr[T]](
	handler CustomHandler[In, T, Out], inputConnector conn[In], outputConnector conn[Out], cfg *CustomConfig,
) *CustomStage[In, T, Out] {

	return &CustomStage[In, T, Out]{
		stage: newStage(
			cfg.Name, inputConnector, outputConnector, newCustomWorkerInstMaker[In, T, Out](), cfg.Stage,
		),

		cfg: cfg,

		handler: handler,
	}
}

// Init initializes the stage.
func (cs *CustomStage[In, T, Out]) Init(ctx context.Context) error {
	// Initialize the handler
	if err := cs.handler.Init(ctx); err != nil {
		return err
	}

	return cs.stage.Init(ctx, newCustomWorkerArgs(cs.cfg.Name, cs.handler))
}
