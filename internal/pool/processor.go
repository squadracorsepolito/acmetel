package pool

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
)

// Processor is a worker pool intended to be used by a processor stage.
type Processor[W, InitArgs any, In, Out message.Message, WPtr ProcessorWorkerPtr[W, InitArgs, In, Out]] struct {
	tel *internal.Telemetry

	cfg *Config

	scaler *scaler

	initArgs InitArgs

	wg *sync.WaitGroup

	fanOut *fanOut[In]
	fanIn  *fanIn[Out]

	handledMessages atomic.Int64
	handlingErrors  atomic.Int64
}

// NewProcessor returns a new processor worker pool.
func NewProcessor[W, InitArgs any, In, Out message.Message, WPtr ProcessorWorkerPtr[W, InitArgs, In, Out]](tel *internal.Telemetry, cfg *Config) *Processor[W, InitArgs, In, Out, WPtr] {
	return &Processor[W, InitArgs, In, Out, WPtr]{
		tel: tel,

		cfg: cfg,

		scaler: newScaler(tel, cfg.toScaler()),

		wg: &sync.WaitGroup{},

		fanOut: newFanOut[In](cfg.InputQueueSize),
		fanIn:  newFanIn[Out](cfg.OutputQueueSize),
	}
}

// Init initialises the worker pool.
func (p *Processor[W, InitArgs, In, Out, WPtr]) Init(ctx context.Context, initArgs InitArgs) error {
	p.initMetrics()

	p.initArgs = initArgs
	p.scaler.init(ctx, p.cfg.InitialWorkers)

	return nil
}

func (p *Processor[W, InitArgs, In, Out, WPtr]) initMetrics() {
	p.tel.NewCounter("worker_pool_handled_messages", func() int64 { return p.handledMessages.Load() })
}

// Run runs the worker pool.
func (p *Processor[W, InitArgs, In, Out, WPtr]) Run(ctx context.Context) {
	go p.runStartWorkerListener(ctx)
	go p.scaler.run(ctx)
}

func (p *Processor[W, InitArgs, In, Out, WPtr]) runStartWorkerListener(ctx context.Context) {
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

func (p *Processor[W, InitArgs, In, Out, WPtr]) runWorker(ctx context.Context) {
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
	if stopCh == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return

		case <-stopCh:
			return

		default:
			msgIn, err := p.fanOut.readTask()
			if err != nil {
				continue
			}

			msgOut, err := worker.Handle(ctx, msgIn)
			if err != nil {
				p.tel.LogError("failed to do work", err, "worker_id", workerID)
				p.handlingErrors.Add(1)

				goto loopCleanup
			}

			if msgOut.IsDropped() {
				goto loopCleanup
			}

			// Set the receive time and timestamp
			msgOut.SetReceiveTime(msgIn.GetReceiveTime())
			msgOut.SetTimestamp(msgIn.GetTimestamp())

			p.handledMessages.Add(1)

			if err := p.fanIn.addTask(msgOut); err != nil {
				continue
			}

		loopCleanup:
			msgIn.Destroy()
			p.scaler.notifyTaskCompleted()
		}
	}
}

// Close closes the worker pool.
func (p *Processor[W, InitArgs, In, Out, WPtr]) Close() {
	p.tel.LogInfo("closing worker pool")

	p.fanOut.close()

	p.wg.Wait()
	p.scaler.stop()

	p.fanIn.close()
}

// AddMessage adds a new task to the worker pool queue.
func (p *Processor[W, InitArgs, In, Out, WPtr]) AddMessage(ctx context.Context, msgIn In) error {
	if err := p.fanOut.addTask(ctx, msgIn); err != nil {
		return err
	}

	p.scaler.notifyTaskAdded()

	return nil
}

// ExtractMessage extracts a message from the worker pool output queue.
func (p *Processor[W, InitArgs, In, Out, WPtr]) ExtractMessage() (Out, error) {
	return p.fanIn.readTask()
}
