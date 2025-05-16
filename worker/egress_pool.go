package worker

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmetel/internal"
)

type EgressPool[W, InitArgs, In any, WPtr EgressWorkerPtr[W, InitArgs, In]] struct {
	l *internal.Logger

	cfg *PoolConfig

	scaler *scaler

	initArgs InitArgs

	wg *sync.WaitGroup

	inputCh chan In
}

func NewEgressPool[W, InitArgs, In any, WPtr EgressWorkerPtr[W, InitArgs, In]](l *internal.Logger, cfg *PoolConfig) *EgressPool[W, InitArgs, In, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8

	return &EgressPool[W, InitArgs, In, WPtr]{
		l: l,

		cfg: cfg,

		scaler: newScaler(l, cfg.toScaler()),

		wg: &sync.WaitGroup{},

		inputCh: make(chan In, channelSize),
	}
}

func (ep *EgressPool[W, InitArgs, In, WPtr]) Init(ctx context.Context, initArgs InitArgs) error {
	ep.initArgs = initArgs

	ep.scaler.init(ctx, ep.cfg.InitialWorkers)

	return nil
}

func (ep *EgressPool[W, InitArgs, In, WPtr]) Run(ctx context.Context) {
	go ep.runStartWorkerListener(ctx)
	go ep.scaler.run(ctx)
}

func (ep *EgressPool[W, InitArgs, In, WPtr]) runStartWorkerListener(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-ep.scaler.getStartCh():
			go ep.runWorker(ctx)
		}
	}
}

func (ep *EgressPool[W, InitArgs, In, WPtr]) runWorker(ctx context.Context) {
	var dummyWorker W
	worker := WPtr(&dummyWorker)

	if err := worker.Init(ctx, ep.initArgs); err != nil {
		ep.l.Error("failed to init worker", err)
		return
	}

	ep.wg.Add(1)
	defer ep.wg.Done()

	workerID := ep.scaler.notifyWorkerStart()
	defer ep.scaler.notifyWorkerStop()

	ep.l.Info("starting worker", "worker_id", workerID)

	defer func() {
		ep.l.Info("stopping worker", "worker_id", workerID)

		if err := worker.Stop(ctx); err != nil {
			ep.l.Error("failed to stop worker", err, "worker_id", workerID)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ep.scaler.getStopCh(workerID):
			return

		case dataIn := <-ep.inputCh:
			if err := worker.DoWork(ctx, dataIn); err != nil {
				ep.l.Error("failed to do work", err, "worker_id", workerID)
				continue
			}

			ep.scaler.notifyTaskCompleted()
		}
	}
}

func (ep *EgressPool[W, InitArgs, In, WPtr]) Stop() {
	ep.l.Info("stopping worker pool")

	ep.wg.Wait()
	ep.scaler.stop()

	close(ep.inputCh)
}

func (ep *EgressPool[W, InitArgs, In, WPtr]) AddTask(ctx context.Context, task In) bool {
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
