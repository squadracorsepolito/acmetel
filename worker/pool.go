package worker

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
)

type Pool[W, InitArgs any, In, Out message.Message, WPtr HandlerWorkerPtr[W, InitArgs, In, Out]] struct {
	*withOutput[Out]

	tel *internal.Telemetry

	cfg *PoolConfig

	scaler *scaler

	initArgs InitArgs

	wg *sync.WaitGroup

	inputCh chan In

	handledMessages atomic.Int64
	handlingErrors  atomic.Int64
}

func NewPool[W, InitArgs any, In, Out message.Message, WPtr HandlerWorkerPtr[W, InitArgs, In, Out]](tel *internal.Telemetry, cfg *PoolConfig) *Pool[W, InitArgs, In, Out, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8 * 32

	return &Pool[W, InitArgs, In, Out, WPtr]{
		withOutput: newWithOutput[Out](channelSize),

		tel: tel,

		cfg: cfg,

		scaler: newScaler(tel, cfg.toScaler()),

		wg: &sync.WaitGroup{},

		inputCh: make(chan In, channelSize),
	}
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) Init(ctx context.Context, initArgs InitArgs) error {
	p.initMetrics()

	p.initArgs = initArgs
	p.scaler.init(ctx, p.cfg.InitialWorkers)

	return nil
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) initMetrics() {
	p.tel.NewCounter("worker_pool_handled_messages", func() int64 { return p.handledMessages.Load() })
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) Run(ctx context.Context) {
	go p.runStartWorkerListener(ctx)
	go p.scaler.run(ctx)
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) runStartWorkerListener(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-p.scaler.getStartCh():
			go p.runWorker(ctx)
		}
	}
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) runWorker(ctx context.Context) {
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

		if err := worker.Stop(ctx); err != nil {
			p.tel.LogError("failed to stop worker", err, "worker_id", workerID)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-p.scaler.getStopCh(workerID):
			return

		case msgIn := <-p.inputCh:
			tracedCtx, span := p.tel.NewTrace(msgIn.LoadSpanContext(ctx), "handle message")

			receiveTime := msgIn.ReceiveTime()

			msgOut, err := worker.Handle(tracedCtx, msgIn)
			if err != nil {
				p.tel.LogError("failed to do work", err, "worker_id", workerID)
				p.handlingErrors.Add(1)

				goto loopCleanup
			}

			msgOut.SetReceiveTime(receiveTime)

			p.handledMessages.Add(1)
			msgOut.SaveSpan(span)

			p.sendOutput(msgOut)

			span.AddEvent("message sent to next stage")

		loopCleanup:
			p.scaler.notifyTaskCompleted()
			span.End()
		}
	}
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) Stop() {
	p.tel.LogInfo("stopping worker pool")

	p.wg.Wait()
	p.scaler.stop()

	close(p.inputCh)

	p.closeOutput()
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) AddTask(ctx context.Context, task In) bool {
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
