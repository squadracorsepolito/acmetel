package worker

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmetel/internal"
)

type Pool[W, InitArgs, In, Out any, WPtr WorkerPtr[W, InitArgs, In, Out]] struct {
	l *internal.Logger

	cfg *PoolConfig

	scaler *scaler

	initArgs InitArgs

	wg *sync.WaitGroup

	inputCh  chan In
	outputCh chan Out
}

func NewPool[W, InitArgs, In, Out any, WPtr WorkerPtr[W, InitArgs, In, Out]](l *internal.Logger, cfg *PoolConfig) *Pool[W, InitArgs, In, Out, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8

	return &Pool[W, InitArgs, In, Out, WPtr]{
		l: l,

		cfg: cfg,

		scaler: newScaler(l, cfg.toScaler()),

		wg: &sync.WaitGroup{},

		inputCh:  make(chan In, channelSize),
		outputCh: make(chan Out, channelSize),
	}
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) Init(ctx context.Context, initArgs InitArgs) error {
	p.initArgs = initArgs

	p.scaler.init(ctx, p.cfg.InitialWorkers)

	return nil
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

	if err := worker.Init(ctx, p.initArgs); err != nil {
		p.l.Error("failed to init worker", err)
		return
	}

	p.wg.Add(1)
	defer p.wg.Done()

	workerID := p.scaler.notifyWorkerStart()
	defer p.scaler.notifyWorkerStop()

	p.l.Info("starting worker", "worker_id", workerID)

	defer func() {
		p.l.Info("stopping worker", "worker_id", workerID)

		if err := worker.Stop(ctx); err != nil {
			p.l.Error("failed to stop worker", err, "worker_id", workerID)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-p.scaler.getStopCh(workerID):
			return

		case dataIn := <-p.inputCh:
			res, err := worker.DoWork(ctx, dataIn)
			if err != nil {
				p.l.Error("failed to do work", err, "worker_id", workerID)
				continue
			}

			p.scaler.notifyTaskCompleted()

			p.outputCh <- res
		}
	}
}

func (p *Pool[W, InitArgs, In, Out, WPtr]) Stop() {
	p.l.Info("stopping worker pool")

	p.wg.Wait()
	p.scaler.stop()

	close(p.inputCh)
	close(p.outputCh)
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

func (p *Pool[W, InitArgs, In, Out, WPtr]) GetOutputCh() <-chan Out {
	return p.outputCh
}
