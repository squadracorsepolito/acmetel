package internal

import (
	"context"
	"sync"
)

type WorkerPoolConfig struct {
	WorkerNum   int
	ChannelSize int
}

type WorkerGen[In, Out any] func() Worker[In, Out]

type Worker[In, Out any] interface {
	Logger() *Logger

	SetID(id int)
	ID() int

	DoWork(ctx context.Context, dataIn In) (Out, error)
}

type BaseWorker struct {
	l  *Logger
	id int
}

func NewBaseWorker(l *Logger) *BaseWorker {
	return &BaseWorker{
		l: l,
	}
}

func (w *BaseWorker) Logger() *Logger {
	return w.l
}

func (w *BaseWorker) SetID(id int) {
	w.id = id
}

func (w *BaseWorker) ID() int {
	return w.id
}

type WorkerPool[In, Out any] struct {
	num     int
	wg      *sync.WaitGroup
	workers []Worker[In, Out]

	InputCh  chan In
	OutputCh chan Out
}

func NewWorkerPool[In, Out any](workerNum, channelSize int, workerGen WorkerGen[In, Out]) *WorkerPool[In, Out] {
	workers := make([]Worker[In, Out], workerNum)
	for i := range workerNum {
		worker := workerGen()
		worker.SetID(i)
		workers[i] = worker
	}

	return &WorkerPool[In, Out]{
		num:     workerNum,
		wg:      &sync.WaitGroup{},
		workers: workers,

		InputCh:  make(chan In, channelSize),
		OutputCh: make(chan Out, channelSize),
	}
}

func (wp *WorkerPool[In, Out]) runWorker(ctx context.Context, worker Worker[In, Out]) {
	defer wp.wg.Done()

	worker.Logger().Info("starting worker", "worker_id", worker.ID())
	defer worker.Logger().Info("stopping worker", "worker_id", worker.ID())

	for {
		select {
		case <-ctx.Done():
			return
		case dataIn := <-wp.InputCh:
			dataOut, err := worker.DoWork(ctx, dataIn)
			if err != nil {
				worker.Logger().Error("failed to do work", "worker_id", worker.ID(), "cause", err)
				continue
			}

			wp.OutputCh <- dataOut
		}
	}
}

func (wp *WorkerPool[In, Out]) Run(ctx context.Context) {
	wp.wg.Add(wp.num)

	for _, worker := range wp.workers {
		go wp.runWorker(ctx, worker)
	}
}

func (wp *WorkerPool[In, Out]) Stop() {
	wp.wg.Wait()
	close(wp.InputCh)
	close(wp.OutputCh)
}
