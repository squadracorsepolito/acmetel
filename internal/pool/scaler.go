package pool

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
)

type scalerCfg struct {
	enabled             bool
	minWorkers          int
	maxWorkers          int
	queueDepthThreshold float64
	scaleDownFactor     float64
	scaleDownBackoff    float64
	interval            time.Duration
}

type scaler struct {
	tel *internal.Telemetry

	cfg *scalerCfg

	consecuriveScaleDown int
	scaleDownAt          float64

	startCh    chan struct{}
	stopChList []chan struct{}

	currWorkers   atomic.Int32
	activeWorkers atomic.Int32

	pendingTasks atomic.Int64
}

func newScaler(tel *internal.Telemetry, cfg *scalerCfg) *scaler {
	return &scaler{
		tel: tel,

		cfg: cfg,

		consecuriveScaleDown: 0,
		scaleDownAt:          1,

		startCh:    make(chan struct{}, cfg.maxWorkers),
		stopChList: make([]chan struct{}, 0, cfg.maxWorkers),
	}
}

func (s *scaler) initMetrics() {
	s.tel.NewUpDownCounter("worker_pool_pending_tasks", func() int64 {
		return s.pendingTasks.Load()
	})

	s.tel.NewUpDownCounter("worker_pool_active_workers", func() int64 {
		return int64(s.activeWorkers.Load())
	})
}

func (s *scaler) init(ctx context.Context, initialWorkers int) {
	for range s.cfg.maxWorkers {
		s.stopChList = append(s.stopChList, make(chan struct{}))
	}

	for range initialWorkers {
		s.sendStart(ctx)
	}

	s.currWorkers.Store(int32(initialWorkers))

	s.initMetrics()
}

func (s *scaler) run(ctx context.Context) {
	if !s.cfg.enabled {
		return
	}

	ticker := time.NewTicker(s.cfg.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			s.evaluateAndScale(ctx)
		}
	}
}

func (s *scaler) evaluateAndScale(ctx context.Context) {
	currWorkers := int(s.currWorkers.Load())
	pendingTasks := int(s.pendingTasks.Load())
	activeWorkers := int(s.activeWorkers.Load())

	// Calculate queue depth per worker
	queueDepthPerWorker := float64(pendingTasks) / float64(currWorkers)

	s.tel.LogInfo("auto-scaling metrics",
		"active_workers", activeWorkers,
		"pending_tasks", pendingTasks,
		"queue_depth_per_worker", queueDepthPerWorker,
	)

	// Scale up if queue depth per worker is higher than target
	if queueDepthPerWorker > s.cfg.queueDepthThreshold {
		// Calculate how many workers to add based on queue depth
		workersToAdd := max(int(math.Ceil(float64(pendingTasks)/s.cfg.queueDepthThreshold)), 1)
		targetWorkers := min(currWorkers+workersToAdd, s.cfg.maxWorkers)

		if targetWorkers > currWorkers {
			s.tel.LogInfo("scaling up", "from", currWorkers, "to", targetWorkers)
			s.scaleWorkers(ctx, int(targetWorkers))
		}

		s.resetScaleDownTiming()

		return
	}

	// Scale down if we have more than min workers and there are fewer pending tasks than workers
	if currWorkers > s.cfg.minWorkers && pendingTasks < currWorkers {
		// Check if it is the right time to scale down
		if !s.checkScaleDownTiming() {
			return
		}

		// Remove workers
		workersToRemove := max(int(math.Ceil(float64(currWorkers)*s.cfg.scaleDownFactor)), 1)
		targetWorkers := max(currWorkers-workersToRemove, s.cfg.minWorkers)

		if targetWorkers < currWorkers {
			s.tel.LogInfo("scaling down", "from", currWorkers, "to", targetWorkers)
			s.scaleWorkers(ctx, int(targetWorkers))
		}
	}
}

func (s *scaler) resetScaleDownTiming() {
	s.consecuriveScaleDown = 0
	s.scaleDownAt = 1
}

// checkScaleDownTiming states if it is the right time to scale down
// and updates the necessary parameters
func (s *scaler) checkScaleDownTiming() bool {
	s.consecuriveScaleDown++

	// Check if it is the right time to scale down
	if float64(s.consecuriveScaleDown) < s.scaleDownAt {
		return false
	}

	// Exponentially increase the time to scale down so
	// it gets harder to scale down when multiple consecutive
	// scales down are triggered (exponential backoff)
	nextTime := s.scaleDownAt * s.cfg.scaleDownBackoff
	s.scaleDownAt = min(nextTime, 15)

	return true
}

func (s *scaler) sendStart(ctx context.Context) {
	select {
	case <-ctx.Done():
	case s.startCh <- struct{}{}:
	}
}

func (s *scaler) sendStop(ctx context.Context, id int) {
	if id > s.cfg.maxWorkers {
		return
	}

	select {
	case <-ctx.Done():
	case s.stopChList[id] <- struct{}{}:
	}
}

func (s *scaler) scaleWorkers(ctx context.Context, targetCount int) {
	currWorkerCount := int(s.currWorkers.Swap(int32(targetCount)))
	delta := targetCount - currWorkerCount

	if delta == 0 {
		return
	}

	// Check if it has to scale up worker
	if delta > 0 {
		for range delta {
			s.sendStart(ctx)
		}

		return
	}

	// Scale down
	for i := currWorkerCount - 1; i >= targetCount; i-- {
		s.sendStop(ctx, i)
	}
}

func (s *scaler) stop() {
	for _, stopCh := range s.stopChList {
		close(stopCh)
	}

	close(s.startCh)
}

func (s *scaler) notifyWorkerStart() int {
	workerID := int(s.activeWorkers.Add(1)) - 1
	return workerID
}

func (s *scaler) notifyWorkerStop() {
	s.activeWorkers.Add(-1)
}

func (s *scaler) notifyTaskAdded() {
	s.pendingTasks.Add(1)
}

func (s *scaler) notifyTaskCompleted() {
	s.pendingTasks.Add(-1)
}

func (s *scaler) getStopCh(workerID int) <-chan struct{} {
	return s.stopChList[workerID]
}

func (s *scaler) getStartCh() <-chan struct{} {
	return s.startCh
}
