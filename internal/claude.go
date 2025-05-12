package internal

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// Task represents a job to be executed by a worker
type Task struct {
	ID int
}

// Result represents the outcome of a task
type Result struct {
	TaskID int
	Value  float64
}

// WorkerPool manages a pool of workers that can be scaled up or down
type WorkerPool1 struct {
	tasks             chan Task
	results           chan Result
	wg                sync.WaitGroup
	workerCount       int32
	ctx               context.Context
	cancel            context.CancelFunc
	maxWorkers        int
	minWorkers        int
	activeWorkers     int32
	processingTime    time.Duration // Simulates CPU-bound work duration
	pendingTasks      int32         // Count of tasks waiting to be processed
	completedTasks    int32         // Count of tasks completed
	autoScaleInterval time.Duration // How often to check for scaling needs
	taskLatency       time.Duration // Average time to process a task
	latencyHistory    []time.Duration
	latencyMutex      sync.Mutex
}

// NewWorkerPool creates a new worker pool with initial worker count
func NewWorkerPool1(initialWorkers, minWorkers, maxWorkers int, processingTime time.Duration) *WorkerPool1 {
	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool1{
		tasks:             make(chan Task, 100),
		results:           make(chan Result, 100),
		workerCount:       int32(initialWorkers),
		ctx:               ctx,
		cancel:            cancel,
		minWorkers:        minWorkers,
		maxWorkers:        maxWorkers,
		processingTime:    processingTime,
		autoScaleInterval: 2 * time.Second,
		latencyHistory:    make([]time.Duration, 0, 100),
	}
}

// Start initializes the worker pool with the initial number of workers
func (wp *WorkerPool1) Start() {
	fmt.Printf("Starting worker pool with %d workers (min: %d, max: %d)\n",
		atomic.LoadInt32(&wp.workerCount), wp.minWorkers, wp.maxWorkers)

	// Start initial workers
	for i := 0; i < int(atomic.LoadInt32(&wp.workerCount)); i++ {
		wp.startWorker()
	}

	// Start a goroutine to collect and process results
	go wp.processResults()

	// Start auto-scaling goroutine
	go wp.autoScale()
}

// startWorker launches a new worker goroutine
func (wp *WorkerPool1) startWorker() {
	wp.wg.Add(1)
	atomic.AddInt32(&wp.activeWorkers, 1)
	workerID := atomic.LoadInt32(&wp.activeWorkers)

	fmt.Printf("Starting worker #%d\n", workerID)

	go func(id int32) {
		defer wp.wg.Done()
		defer atomic.AddInt32(&wp.activeWorkers, -1)

		for {
			select {
			case <-wp.ctx.Done():
				fmt.Printf("Worker #%d shutting down\n", id)
				return
			case task, ok := <-wp.tasks:
				if !ok {
					fmt.Printf("Worker #%d: task channel closed\n", id)
					return
				}

				// Check if this worker should shut down (for scaling down)
				currentWorkerCount := atomic.LoadInt32(&wp.workerCount)
				if id > currentWorkerCount {
					fmt.Printf("Worker #%d scaling down\n", id)
					return
				}

				// Perform CPU-bound work
				result := wp.processCPUBoundTask(task)

				// Send result back
				select {
				case <-wp.ctx.Done():
					return
				case wp.results <- result:
					// Result sent successfully
				}
			}
		}
	}(workerID)
}

// ScaleWorkers adjusts the number of workers in the pool
func (wp *WorkerPool1) ScaleWorkers(targetCount int) {
	if targetCount < 1 {
		targetCount = 1 // Ensure at least one worker
	}

	if targetCount > wp.maxWorkers {
		targetCount = wp.maxWorkers // Cap at max workers
	}

	currentCount := int(atomic.LoadInt32(&wp.workerCount))
	delta := targetCount - currentCount

	if delta > 0 {
		// Scale up
		fmt.Printf("Scaling up: adding %d workers\n", delta)
		atomic.StoreInt32(&wp.workerCount, int32(targetCount))
		for i := 0; i < delta; i++ {
			wp.startWorker()
		}
	} else if delta < 0 {
		// Scale down happens naturally as workers check the workerCount
		fmt.Printf("Scaling down: removing %d workers\n", -delta)
		atomic.StoreInt32(&wp.workerCount, int32(targetCount))
		// Existing workers will exit based on the new worker count
	}
}

// processCPUBoundTask simulates a CPU-bound operation
func (wp *WorkerPool1) processCPUBoundTask(task Task) Result {
	start := time.Now()

	// Simulate CPU-intensive work by performing complex calculations
	value := 0.0
	iterations := int(wp.processingTime.Milliseconds()) * 1000 // Scale with desired processing time

	for i := 0; i < iterations; i++ {
		x := rand.Float64() * 10
		value += math.Sin(x) * math.Cos(x) / math.Sqrt(x+1)
	}

	elapsed := time.Since(start)
	fmt.Printf("Task #%d completed in %v\n", task.ID, elapsed)

	// Record task latency for scaling decisions
	wp.latencyMutex.Lock()
	wp.latencyHistory = append(wp.latencyHistory, elapsed)
	// Keep the history at a reasonable size
	if len(wp.latencyHistory) > 100 {
		wp.latencyHistory = wp.latencyHistory[1:]
	}
	wp.taskLatency = wp.calculateAverageLatency()
	wp.latencyMutex.Unlock()

	atomic.AddInt32(&wp.pendingTasks, -1)
	atomic.AddInt32(&wp.completedTasks, 1)

	return Result{
		TaskID: task.ID,
		Value:  value,
	}
}

// AddTask submits a new task to the worker pool
func (wp *WorkerPool1) AddTask(task Task) {
	atomic.AddInt32(&wp.pendingTasks, 1)

	select {
	case <-wp.ctx.Done():
		atomic.AddInt32(&wp.pendingTasks, -1)
		return
	case wp.tasks <- task:
		// Task added successfully
	}
}

// processResults handles the results produced by workers
func (wp *WorkerPool1) processResults() {
	for {
		select {
		case <-wp.ctx.Done():
			return
		case result, ok := <-wp.results:
			if !ok {
				return
			}
			fmt.Printf("Result from task #%d: %.4f\n", result.TaskID, result.Value)
		}
	}
}

// Stop gracefully shuts down the worker pool
func (wp *WorkerPool1) Stop() {
	fmt.Println("Shutting down worker pool...")
	wp.cancel()
	close(wp.tasks)
	wp.wg.Wait()
	close(wp.results)
	fmt.Println("Worker pool shutdown complete")
}

// autoScale automatically adjusts the number of workers based on workload
func (wp *WorkerPool1) autoScale() {
	ticker := time.NewTicker(wp.autoScaleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case <-ticker.C:
			wp.evaluateAndScale()
		}
	}
}

// calculateAverageLatency computes the average processing time of recent tasks
func (wp *WorkerPool1) calculateAverageLatency() time.Duration {
	if len(wp.latencyHistory) == 0 {
		return wp.processingTime // Default to expected processing time if no history
	}

	var total time.Duration
	for _, latency := range wp.latencyHistory {
		total += latency
	}
	return total / time.Duration(len(wp.latencyHistory))
}

// evaluateAndScale decides whether to scale up or down based on workload metrics
func (wp *WorkerPool1) evaluateAndScale() {
	currentWorkers := atomic.LoadInt32(&wp.workerCount)
	pendingTasks := atomic.LoadInt32(&wp.pendingTasks)
	activeWorkers := atomic.LoadInt32(&wp.activeWorkers)
	avgLatency := wp.taskLatency

	// Calculate queue depth per worker
	queueDepthPerWorker := float64(pendingTasks) / float64(currentWorkers)

	// Calculate utilization (active workers as percentage of total workers)
	utilization := float64(activeWorkers) / float64(currentWorkers)

	// Log current metrics
	fmt.Printf("\nAuto-scaling metrics:\n"+
		"  Workers: %d/%d (min: %d, max: %d)\n"+
		"  Pending tasks: %d\n"+
		"  Queue depth per worker: %.2f\n"+
		"  Worker utilization: %.2f%%\n"+
		"  Avg task latency: %v\n",
		activeWorkers, currentWorkers, wp.minWorkers, wp.maxWorkers,
		pendingTasks, queueDepthPerWorker, utilization*100, avgLatency)

	// Scaling logic
	var targetWorkers int32

	// Scale up if:
	// 1. Queue depth per worker is high OR
	// 2. Utilization is high (> 80%) and we're not at max workers
	if queueDepthPerWorker > 3.0 || (utilization > 0.8 && currentWorkers < int32(wp.maxWorkers)) {
		// Calculate how many workers to add based on queue depth
		workersToAdd := int32(math.Ceil(float64(pendingTasks) / 3.0)) // Target 3 tasks per worker

		if workersToAdd < 1 {
			workersToAdd = 1 // Add at least one worker
		}

		targetWorkers = currentWorkers + workersToAdd
		if targetWorkers > int32(wp.maxWorkers) {
			targetWorkers = int32(wp.maxWorkers)
		}

		if targetWorkers > currentWorkers {
			fmt.Printf("Auto-scaling UP from %d to %d workers\n", currentWorkers, targetWorkers)
			wp.ScaleWorkers(int(targetWorkers))
		}
	} else if utilization < 0.3 && currentWorkers > int32(wp.minWorkers) && pendingTasks < currentWorkers {
		// Scale down if utilization is low (< 30%) and we have more than min workers
		// and there are fewer pending tasks than workers
		workersToRemove := int32(math.Ceil(float64(currentWorkers) * 0.2)) // Remove up to 20% of workers

		if workersToRemove < 1 {
			workersToRemove = 1 // Remove at least one worker
		}

		targetWorkers = currentWorkers - workersToRemove
		if targetWorkers < int32(wp.minWorkers) {
			targetWorkers = int32(wp.minWorkers)
		}

		if targetWorkers < currentWorkers {
			fmt.Printf("Auto-scaling DOWN from %d to %d workers\n", currentWorkers, targetWorkers)
			wp.ScaleWorkers(int(targetWorkers))
		}
	}
}

// func main() {
// 	// Create a worker pool with initial, min, and max workers
// 	cpuCount := runtime.NumCPU()
// 	initialWorkers := cpuCount / 2      // Start with half the available CPUs
// 	minWorkers := 1                     // At least one worker at all times
// 	maxWorkers := cpuCount * 2          // Allow scaling up to twice the CPUs
// 	processingTime := 500 * time.Millisecond

// 	pool := NewWorkerPool1(initialWorkers, minWorkers, maxWorkers, processingTime)
// 	pool.Start()

// 	// Set up signal handling for graceful shutdown
// 	sigChan := make(chan os.Signal, 1)
// 	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

// 	// Submit tasks in a separate goroutine with varying load patterns
// 	go func() {
// 		// First batch: steady load
// 		fmt.Println("\n--- Phase 1: Steady Load ---")
// 		for i := 1; i <= 30; i++ {
// 			pool.AddTask(Task{ID: i})
// 			time.Sleep(300 * time.Millisecond)
// 		}

// 		// Second batch: high load (burst)
// 		fmt.Println("\n--- Phase 2: High Load Burst ---")
// 		for i := 31; i <= 60; i++ {
// 			pool.AddTask(Task{ID: i})
// 			time.Sleep(50 * time.Millisecond) // Faster submission rate
// 		}

// 		// Wait for the tasks to be processed
// 		time.Sleep(5 * time.Second)

// 		// Third batch: low load
// 		fmt.Println("\n--- Phase 3: Low Load ---")
// 		for i := 61; i <= 75; i++ {
// 			pool.AddTask(Task{ID: i})
// 			time.Sleep(1 * time.Second) // Slower submission rate
// 		}

// 		// Final batch: medium load
// 		fmt.Println("\n--- Phase 4: Medium Load ---")
// 		for i := 76; i <= 100; i++ {
// 			pool.AddTask(Task{ID: i})
// 			time.Sleep(200 * time.Millisecond)
// 		}

// 		fmt.Println("\nAll tasks submitted")
// 	}()

// 	// Wait for interrupt signal
// 	<-sigChan
// 	fmt.Println("\nReceived interrupt signal")

// 	// Shut down the pool gracefully
// 	pool.Stop()
// }
