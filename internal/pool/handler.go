package pool

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

// Handler is a worker pool intended to be used by an handler stage.
type Handler[W, InitArgs any, In, Out message.Message, WPtr HandlerWorkerPtr[W, InitArgs, In, Out]] struct {
	*withOutput[Out]

	tel *internal.Telemetry

	cfg *Config

	scaler *scaler

	initArgs InitArgs

	wg *sync.WaitGroup

	inputCh chan In

	handledMessages atomic.Int64
	handlingErrors  atomic.Int64
}

// NewHandler returns a new handler worker pool.
func NewHandler[W, InitArgs any, In, Out message.Message, WPtr HandlerWorkerPtr[W, InitArgs, In, Out]](tel *internal.Telemetry, cfg *Config) *Handler[W, InitArgs, In, Out, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8 * 32

	return &Handler[W, InitArgs, In, Out, WPtr]{
		withOutput: newWithOutput[Out](channelSize),

		tel: tel,

		cfg: cfg,

		scaler: newScaler(tel, cfg.toScaler()),

		wg: &sync.WaitGroup{},

		inputCh: make(chan In, channelSize),
	}
}

// Init initialises the worker pool.
func (p *Handler[W, InitArgs, In, Out, WPtr]) Init(ctx context.Context, initArgs InitArgs) error {
	p.initMetrics()

	p.initArgs = initArgs
	p.scaler.init(ctx, p.cfg.InitialWorkers)

	return nil
}

func (p *Handler[W, InitArgs, In, Out, WPtr]) initMetrics() {
	p.tel.NewCounter("worker_pool_handled_messages", func() int64 { return p.handledMessages.Load() })
}

// Run runs the worker pool.
func (p *Handler[W, InitArgs, In, Out, WPtr]) Run(ctx context.Context) {
	go p.runStartWorkerListener(ctx)
	go p.scaler.run(ctx)
}

func (p *Handler[W, InitArgs, In, Out, WPtr]) runStartWorkerListener(ctx context.Context) {
	startWorkerCh := p.scaler.getStartCh()

	for {
		select {
		case <-ctx.Done():
			return

		case <-startWorkerCh:
			go p.runWorker(ctx)
		}
	}
}

func (p *Handler[W, InitArgs, In, Out, WPtr]) runWorker(ctx context.Context) {
	var dummyWorker W
	worker := WPtr(&dummyWorker)

	worker.SetTelemetry(p.tel)

	if err := worker.Init(ctx, p.initArgs); err != nil {
		p.tel.LogError("failed to init worker", err)
		return
	}

	p.wg.Add(1)
	defer p.wg.Done()

	workerID := p.scaler.notifyWorkerStart()
	defer p.scaler.notifyWorkerStop()

	p.tel.LogInfo("starting worker", "worker_id", workerID)

	defer func() {
		p.tel.LogInfo("stopping worker", "worker_id", workerID)

		if err := worker.Close(ctx); err != nil {
			p.tel.LogError("failed to stop worker", err, "worker_id", workerID)
		}
	}()

	stopCh := p.scaler.getStopCh(workerID)

	for {
		select {
		case <-ctx.Done():
			return

		case <-stopCh:
			return

		case msgIn := <-p.inputCh:
			msgOut, err := worker.Handle(ctx, msgIn)
			if err != nil {
				p.tel.LogError("failed to do work", err, "worker_id", workerID)
				p.handlingErrors.Add(1)

				goto loopCleanup
			}

			p.handledMessages.Add(1)

			p.sendOutput(msgOut)

		loopCleanup:
			p.scaler.notifyTaskCompleted()
		}
	}
}

// Close closes the worker pool.
func (p *Handler[W, InitArgs, In, Out, WPtr]) Close() {
	p.tel.LogInfo("closing worker pool")

	p.wg.Wait()
	p.scaler.stop()

	close(p.inputCh)

	p.closeOutput()
}

// AddTask adds a new task to the worker pool.
func (p *Handler[W, InitArgs, In, Out, WPtr]) AddTask(ctx context.Context, task In) bool {
	select {
	case <-ctx.Done():
		return false

	case p.inputCh <- task:
		p.scaler.notifyTaskAdded()
		return true

	default:
		return false
	}
}
