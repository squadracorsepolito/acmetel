package worker

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
)

type EgressPool[In message.Traceable, W, InitArgs any, WPtr EgressWorkerPtr[W, InitArgs, In]] struct {
	tel *internal.Telemetry

	cfg *PoolConfig

	scaler *scaler

	initArgs InitArgs

	wg *sync.WaitGroup

	inputCh chan In
}

func NewEgressPool[In message.Traceable, W, InitArgs any, WPtr EgressWorkerPtr[W, InitArgs, In]](tel *internal.Telemetry, cfg *PoolConfig) *EgressPool[In, W, InitArgs, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8

	return &EgressPool[In, W, InitArgs, WPtr]{
		tel: tel,

		cfg: cfg,

		scaler: newScaler(tel, cfg.toScaler()),

		wg: &sync.WaitGroup{},

		inputCh: make(chan In, channelSize),
	}
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) Init(ctx context.Context, initArgs InitArgs) error {
	ep.initArgs = initArgs

	ep.scaler.init(ctx, ep.cfg.InitialWorkers)

	return nil
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) Run(ctx context.Context) {
	go ep.runStartWorkerListener(ctx)
	go ep.scaler.run(ctx)
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) runStartWorkerListener(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-ep.scaler.getStartCh():
			go ep.runWorker(ctx)
		}
	}
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) runWorker(ctx context.Context) {
	var dummyWorker W
	worker := WPtr(&dummyWorker)

	worker.SetTelemetry(ep.tel)

	if err := worker.Init(ctx, ep.initArgs); err != nil {
		ep.tel.LogError("failed to init worker", err)
		return
	}

	ep.wg.Add(1)
	defer ep.wg.Done()

	workerID := ep.scaler.notifyWorkerStart()
	defer ep.scaler.notifyWorkerStop()

	ep.tel.LogInfo("starting worker", "worker_id", workerID)

	defer func() {
		ep.tel.LogInfo("stopping worker", "worker_id", workerID)

		if err := worker.Stop(ctx); err != nil {
			ep.tel.LogError("failed to stop worker", err, "worker_id", workerID)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ep.scaler.getStopCh(workerID):
			return

		case msgIn := <-ep.inputCh:
			tracedCtx, span := ep.tel.NewTrace(msgIn.LoadSpanContext(ctx), "deliver message")

			if err := worker.Deliver(tracedCtx, msgIn); err != nil {
				ep.tel.LogError("failed to deliver message", err, "worker_id", workerID)
			}

			ep.scaler.notifyTaskCompleted()
			span.End()
		}
	}
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) Stop() {
	ep.tel.LogInfo("stopping worker pool")

	ep.wg.Wait()
	ep.scaler.stop()

	close(ep.inputCh)
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) AddTask(ctx context.Context, task In) bool {
	select {
	case <-ctx.Done():
		return false

	case ep.inputCh <- task:
		ep.scaler.notifyTaskAdded()
		return true

	default:
		return false
	}
}
