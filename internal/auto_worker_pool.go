package internal

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type Worker0[Task, Result any] interface {
	DoWork(ctx context.Context, task Task) (Result, error)
}

type WorkerGen0[Task, Result any] func() Worker0[Task, Result]

type WorkerPoolConfigs struct {
	InitialWorkers         int
	MinWorkers, MaxWorkers int
	QueueDepthPerWorker    int
	AutoScaleInterval      time.Duration

	ScalingSensivity float64
}

type throughputMetrics struct {
	throughput  float64
	workerCount int
}

type WorkerPool0[Task, Result any] struct {
	l *Logger

	minWorkers        int
	maxWorkers        int
	targetQueueDepth  float64
	autoScaleInterval time.Duration

	workerGen WorkerGen0[Task, Result]
	wg        *sync.WaitGroup

	currWorkers   atomic.Int32
	activeWorkers atomic.Int32

	stopWorkerCh []chan struct{}

	pendingTasks atomic.Int32

	InputCh  chan Task
	OutputCh chan Result

	scalingSensivity float64
}

func NewWP[In, Out any](l *Logger, workerGen WorkerGen0[In, Out], configs WorkerPoolConfigs) *WorkerPool0[In, Out] {
	channelSize := configs.MaxWorkers * configs.QueueDepthPerWorker * 2

	wp := &WorkerPool0[In, Out]{
		l: l,

		minWorkers:        configs.MinWorkers,
		maxWorkers:        configs.MaxWorkers,
		targetQueueDepth:  float64(configs.QueueDepthPerWorker),
		autoScaleInterval: configs.AutoScaleInterval,

		workerGen: workerGen,
		wg:        &sync.WaitGroup{},

		stopWorkerCh: make([]chan struct{}, 0, configs.MaxWorkers),

		InputCh:  make(chan In, channelSize),
		OutputCh: make(chan Out, channelSize),

		scalingSensivity: configs.ScalingSensivity,
	}

	for range configs.MaxWorkers {
		wp.stopWorkerCh = append(wp.stopWorkerCh, make(chan struct{}))
	}

	wp.currWorkers.Store(int32(configs.InitialWorkers))

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
		"curr_workers", currWorkers,
		"pending_tasks", pendingTasks,
		"queue_depth_per_worker", queueDepthPerWorker,
	)

	// Check if we're within an acceptable range based on sensitivity
	acceptableRange := wp.targetQueueDepth * (1 + wp.scalingSensivity)
	minimumRange := wp.targetQueueDepth * (1 - wp.scalingSensivity)

	if queueDepthPerWorker <= acceptableRange && queueDepthPerWorker >= minimumRange {
		return
	}

	// Intelligent scaling calculation
	var targetWorkers int

	// Scale up strategy
	if queueDepthPerWorker > acceptableRange {
		// More precise scaling calculation
		targetWorkers = int(math.Ceil(float64(pendingTasks) / wp.targetQueueDepth))

		// Ensure we don't over-scale
		targetWorkers = min(targetWorkers, wp.maxWorkers)

		// Gradual scaling: add workers incrementally
		additionalWorkers := max(targetWorkers-currWorkers, 1)
		targetWorkers = min(currWorkers+additionalWorkers, wp.maxWorkers)
	}

	// Scale down strategy
	if queueDepthPerWorker < minimumRange {
		// Calculate potential workers needed based on current load
		targetWorkers = max(int(math.Ceil(float64(pendingTasks)/wp.targetQueueDepth)), wp.minWorkers)

		// Ensure gradual scale-down
		workersToRemove := max(int(math.Ceil(float64(currWorkers)*wp.scalingSensivity)), 1)
		targetWorkers = max(currWorkers-workersToRemove, wp.minWorkers)
	}

	// Only scale if there's a meaningful change
	if targetWorkers != currWorkers {
		wp.l.Info("scaling workers",
			"from", currWorkers,
			"to", targetWorkers,
			"queue_depth", queueDepthPerWorker,
			"target_depth", wp.targetQueueDepth,
		)
		wp.scaleWorkers(ctx, targetWorkers)
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
		case wp.stopWorkerCh[i] <- struct{}{}:
		}
	}
}

func (wp *WorkerPool0[In, Out]) Run(ctx context.Context) {
	for range wp.currWorkers.Load() {
		go wp.runWorker(ctx)
	}

	go wp.runAutoScale(ctx)
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
