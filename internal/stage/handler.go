package stage

import (
	"context"
	"errors"
	"sync"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/rb"
)

type Handler[MIn, MOut msg, W, WArgs any, WPtr handlerWorkerPtr[W, WArgs, MIn, MOut]] struct {
	tel *internal.Telemetry

	inputConnector  connector.Connector[MIn]
	outputConnector connector.Connector[MOut]

	writerWg *sync.WaitGroup

	workerPool *pool.Handler[W, WArgs, MIn, MOut, WPtr]
}

func NewHandler[MIn, MOut msg, W, WArgs any, WPtr handlerWorkerPtr[W, WArgs, MIn, MOut]](
	name string, inputConnector connector.Connector[MIn], outputConnector connector.Connector[MOut], poolCfg *pool.Config,
) *Handler[MIn, MOut, W, WArgs, WPtr] {

	tel := internal.NewTelemetry("handler", name)

	return &Handler[MIn, MOut, W, WArgs, WPtr]{
		tel: tel,

		inputConnector:  inputConnector,
		outputConnector: outputConnector,

		writerWg: &sync.WaitGroup{},

		workerPool: pool.NewHandler[W, WArgs, MIn, MOut, WPtr](tel, poolCfg),
	}
}

func (h *Handler[MIn, MOut, W, WArgs, WPtr]) Init(ctx context.Context, workerArgs WArgs) error {
	defer h.tel.LogInfo("initialized")

	h.workerPool.Init(ctx, workerArgs)

	return nil
}

func (h *Handler[MIn, MOut, W, WArgs, WPtr]) runWriter(ctx context.Context) {
	h.writerWg.Add(1)
	defer h.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgOut, err := h.workerPool.ExtractMessage()
		if err != nil {
			continue
		}

		if err := h.outputConnector.Write(msgOut); err != nil {
			h.tel.LogError("failed to write into output connector", err)
		}
	}
}

func (h *Handler[MIn, MOut, W, WArgs, WPtr]) Run(ctx context.Context) {
	h.tel.LogInfo("running")
	defer h.tel.LogInfo("stopped")

	// Run the worker pool
	go h.workerPool.Run(ctx)

	// Run the writer goroutine
	go h.runWriter(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		msg, err := h.inputConnector.Read()
		if err != nil {
			// Check if the input connector is closed, if so stop
			if errors.Is(err, connector.ErrClosed) {
				h.tel.LogInfo("input connector is closed, stopping")
				return
			}

			if !errors.Is(err, rb.ErrReadTimeout) {
				h.tel.LogError("failed to read from input connector", err)
			}

			continue
		}

		// Push a new task to the worker pool
		if err := h.workerPool.AddMessage(ctx, msg); err != nil {
			h.tel.LogError("failed to add message to worker pool", err)
			continue
		}
	}
}

func (s *Handler[MIn, MOut, W, WArgs, WPtr]) Close() {
	s.tel.LogInfo("closing")

	// Close the output connector
	s.outputConnector.Close()
	s.workerPool.Close()

	// Wait for the writer to finish
	s.writerWg.Wait()
}

// type HandlerWithROB[MIn msg, MOut reOrdMsg, W, WArgs any, WPtr handlerWorkerPtr[W, WArgs, MIn, MOut]] struct {
// 	*Handler[MIn, MOut, W, WArgs, WPtr]

// 	rob        *rob.ROB[MOut]
// 	robTimeout time.Duration

// 	robOutputCh <-chan MOut

// 	robWg *sync.WaitGroup

// 	// Telemetry metrics
// 	droppedMessages atomic.Int64
// }

// func NewHandlerWithROB[MIn msg, MOut reOrdMsg, W, WArgs any, WPtr handlerWorkerPtr[W, WArgs, MIn, MOut]](
// 	name string, inputConnector connector.Connector[MIn], outputConnector connector.Connector[MOut],
// 	poolCfg *pool.Config, robCfg *rob.Config, robTimeout time.Duration,
// ) *HandlerWithROB[MIn, MOut, W, WArgs, WPtr] {
// 	robCfg.OutputChannelSize = poolCfg.OutputQueueSize

// 	return &HandlerWithROB[MIn, MOut, W, WArgs, WPtr]{
// 		Handler: NewHandler[MIn, MOut, W, WArgs, WPtr](name, inputConnector, outputConnector, poolCfg),

// 		rob:        rob.NewROB[MOut](robCfg),
// 		robTimeout: robTimeout,

// 		robWg: &sync.WaitGroup{},
// 	}
// }

// func (h *HandlerWithROB[MIn, MOut, W, WArgs, WPtr]) initMetrics() {
// 	h.tel.NewCounter("dropped_messages", func() int64 { return h.droppedMessages.Load() })
// }

// func (h *HandlerWithROB[MIn, MOut, W, WArgs, WPtr]) Init(ctx context.Context, workerArgs WArgs) error {
// 	if err := h.Handler.Init(ctx, workerArgs); err != nil {
// 		return err
// 	}

// 	h.robOutputCh = h.rob.GetOutputCh()

// 	h.initMetrics()

// 	return nil
// }

// func (h *HandlerWithROB[MIn, MOut, W, WArgs, WPtr]) runROB(ctx context.Context) {
// 	h.robWg.Add(1)
// 	defer h.robWg.Done()

// 	flushTimeout := time.NewTimer(h.robTimeout)
// 	defer flushTimeout.Stop()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			// Context is done, flush the ROB and return
// 			h.rob.FlushAndReset()
// 			return

// 		case <-flushTimeout.C:
// 			// Timeout expired, flush the ROB
// 			h.rob.FlushAndReset()

// 		default:
// 			msgOut, err := h.workerPool.ExtractMessage()
// 			if err != nil {
// 				continue
// 			}

// 			// Try to enqueue the message
// 			if err := h.rob.Enqueue(msgOut); err != nil {
// 				h.tel.LogError("message dropped", err, "sequence_number", msgOut.GetSequenceNumber())
// 				h.droppedMessages.Add(1)
// 			}
// 		}

// 		// Reset the timeout
// 		flushTimeout.Reset(h.robTimeout)
// 	}
// }

// func (h *HandlerWithROB[MIn, MOut, W, WArgs, WPtr]) runWriter(ctx context.Context) {
// 	h.writerWg.Add(1)
// 	defer h.writerWg.Done()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return
// 		case msgOut := <-h.robOutputCh:
// 			if err := h.outputConnector.Write(msgOut); err != nil {
// 				h.tel.LogError("failed to write into output connector", err)
// 			}
// 		}
// 	}
// }

// func (h *HandlerWithROB[MIn, MOut, W, WArgs, WPtr]) Run(ctx context.Context) {
// 	h.tel.LogInfo("running")
// 	defer h.tel.LogInfo("stopped")

// 	// Run the worker pool
// 	go h.workerPool.Run(ctx)

// 	// Run the writer goroutine
// 	go h.runWriter(ctx)

// 	go h.runROB(ctx)

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return

// 		default:
// 		}

// 		msg, err := h.inputConnector.Read()
// 		if err != nil {
// 			// Check if the input connector is closed, if so stop
// 			if errors.Is(err, connector.ErrClosed) {
// 				h.tel.LogInfo("input connector is closed, stopping")
// 				return
// 			}

// 			h.tel.LogError("failed to read from input connector", err)
// 			continue
// 		}

// 		// Push a new task to the worker pool
// 		if err := h.workerPool.AddMessage(ctx, msg); err != nil {
// 			h.tel.LogError("failed to add message to worker pool", err)
// 			continue
// 		}
// 	}
// }

// func (h *HandlerWithROB[MIn, MOut, W, WArgs, WPtr]) Close() {
// 	h.Handler.Close()
// 	h.robWg.Wait()
// }
