package internal

import (
	"context"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Worker0[In, Out any] interface {
	DoWork(ctx context.Context, task In) (Out, error)
}

type WorkerGen0[In, Out any] func() Worker0[In, Out]

type WorkerPoolConfig0 struct {
	AutoScale           bool
	InitialWorkers      int
	MinWorkers          int
	MaxWorkers          int
	QueueDepthPerWorker int
	ScaleDownFactor     float64
	AutoScaleInterval   time.Duration
}

func NewDefaultWorkerPoolConfig() *WorkerPoolConfig0 {
	return &WorkerPoolConfig0{
		AutoScale:           true,
		InitialWorkers:      1,
		MinWorkers:          1,
		MaxWorkers:          runtime.NumCPU(),
		QueueDepthPerWorker: 128,
		ScaleDownFactor:     0.1,
		AutoScaleInterval:   3 * time.Second,
	}
}

type throughputMetrics struct {
	throughput  float64
	workerCount int
}

type WorkerPool0[In, Out any] struct {
	l *Logger

	workerGen WorkerGen0[In, Out]
	wg        *sync.WaitGroup

	currWorkers   atomic.Int32
	activeWorkers atomic.Int32

	stopWorkerCh []chan struct{}

	pendingTasks atomic.Int64

	autoScale         bool
	minWorkers        int
	maxWorkers        int
	targetQueueDepth  float64
	scaleDownFactor   float64
	autoScaleInterval time.Duration

	InputCh  chan In
	OutputCh chan Out
}

func NewWP[In, Out any](l *Logger, workerGen WorkerGen0[In, Out], cfg *WorkerPoolConfig0) *WorkerPool0[In, Out] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8

	wp := &WorkerPool0[In, Out]{
		l: l,

		workerGen: workerGen,
		wg:        &sync.WaitGroup{},

		stopWorkerCh: make([]chan struct{}, 0, cfg.MaxWorkers),

		autoScale:         cfg.AutoScale,
		minWorkers:        cfg.MinWorkers,
		maxWorkers:        cfg.MaxWorkers,
		targetQueueDepth:  float64(cfg.QueueDepthPerWorker),
		scaleDownFactor:   cfg.ScaleDownFactor,
		autoScaleInterval: cfg.AutoScaleInterval,

		InputCh:  make(chan In, channelSize),
		OutputCh: make(chan Out, channelSize),
	}

	for range cfg.MaxWorkers {
		wp.stopWorkerCh = append(wp.stopWorkerCh, make(chan struct{}))
	}

	wp.currWorkers.Store(int32(cfg.InitialWorkers))

	return wp
}

func (wp *WorkerPool0[In, Out]) runWorker(ctx context.Context) {
	wp.wg.Add(1)
	defer wp.wg.Done()

	workerID := int(wp.activeWorkers.Add(1)) - 1
	defer wp.activeWorkers.Add(-1)

	worker := wp.workerGen()

	wp.l.Info("starting worker", "worker_id", workerID)
	defer wp.l.Info("stopping worker", "worker_id", workerID)

	for {
		select {
		case <-ctx.Done():
			return

		case <-wp.stopWorkerCh[workerID]:
			return

		case dataIn := <-wp.InputCh:
			dataOut, err := worker.DoWork(ctx, dataIn)
			if err != nil {
				wp.l.Error("failed to do work", "worker_id", workerID, "cause", err)
				continue
			}

			wp.pendingTasks.Add(-1)

			wp.OutputCh <- dataOut
		}
	}
}

func (wp *WorkerPool0[In, Out]) runAutoScale(ctx context.Context) {
	ticker := time.NewTicker(wp.autoScaleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			wp.evaluateAndScale(ctx)
		}
	}
}

func (wp *WorkerPool0[In, Out]) evaluateAndScale(ctx context.Context) {
	currWorkers := int(wp.currWorkers.Load())
	pendingTasks := int(wp.pendingTasks.Load())
	activeWorkers := int(wp.activeWorkers.Load())

	// Calculate queue depth per worker
	queueDepthPerWorker := float64(pendingTasks) / float64(currWorkers)

	wp.l.Info("auto-scaling metrics",
		"active_workers", activeWorkers,
		"pending_tasks", pendingTasks,
		"queue_depth_per_worker", queueDepthPerWorker,
	)

	// Scale up if queue depth per worker is higher than target
	if queueDepthPerWorker > wp.targetQueueDepth {
		// Calculate how many workers to add based on queue depth
		workersToAdd := max(int(math.Ceil(float64(pendingTasks)/wp.targetQueueDepth)), 1)
		targetWorkers := min(currWorkers+workersToAdd, wp.maxWorkers)

		if targetWorkers > currWorkers {
			wp.l.Info("scaling up", "from", currWorkers, "to", targetWorkers)
			wp.scaleWorkers(ctx, int(targetWorkers))
		}

		return
	}

	// Scale down if we have more than min workers and there are fewer pending tasks than workers
	if currWorkers > wp.minWorkers && pendingTasks < currWorkers {
		// Remove workers
		workersToRemove := max(int(math.Ceil(float64(currWorkers)*wp.scaleDownFactor)), 1)
		targetWorkers := max(currWorkers-workersToRemove, wp.minWorkers)

		if targetWorkers < currWorkers {
			wp.l.Info("scaling down", "from", currWorkers, "to", targetWorkers)
			wp.scaleWorkers(ctx, int(targetWorkers))
		}
	}
}

func (wp *WorkerPool0[In, Out]) scaleWorkers(ctx context.Context, targetCount int) {
	currWorkerCount := int(wp.currWorkers.Load())
	delta := targetCount - currWorkerCount

	if delta == 0 {
		return
	}

	wp.currWorkers.Store(int32(targetCount))

	// Check if it has to scale up worker
	if delta > 0 {
		for range delta {
			go wp.runWorker(ctx)
		}
	}

	// Scale down
	for i := currWorkerCount - 1; i >= targetCount; i-- {
		select {
		case <-ctx.Done():
			return

		case wp.stopWorkerCh[i] <- struct{}{}:
		}
	}
}

func (wp *WorkerPool0[In, Out]) Run(ctx context.Context) {
	for range wp.currWorkers.Load() {
		go wp.runWorker(ctx)
	}

	if wp.autoScale {
		go wp.runAutoScale(ctx)
	}
}

func (wp *WorkerPool0[In, Out]) Stop() {
	wp.l.Info("stopping worker pool")

	wp.wg.Wait()

	for _, stopCh := range wp.stopWorkerCh {
		close(stopCh)
	}

	close(wp.InputCh)
	close(wp.OutputCh)
}

func (wp *WorkerPool0[In, Out]) AddTask(ctx context.Context, task In) {
	wp.pendingTasks.Add(1)

	select {
	case <-ctx.Done():
		wp.pendingTasks.Add(-1)
		return
	default:
	}

	wp.InputCh <- task
}
