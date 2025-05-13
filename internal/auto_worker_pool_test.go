package internal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockWorker struct {
	delay time.Duration
}

func newMockWorkerGen(dalay time.Duration) WorkerGen0[string, string] {
	return func() Worker0[string, string] {
		return &mockWorker{delay: dalay}
	}
}

func (w *mockWorker) DoWork(ctx context.Context, dataIn string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(w.delay):
		return fmt.Sprintf("processed-%s", dataIn), nil
	}
}

func Test_WorkerPool_ScaleUp(t *testing.T) {
	taskDelay := 50 * time.Millisecond
	taskCount := 100

	wpConfig := &WorkerPoolConfig0{
		AutoScale:           true,
		InitialWorkers:      2,
		MinWorkers:          2,
		MaxWorkers:          20,
		QueueDepthPerWorker: 2,
		ScaleDownFactor:     0.1,
		AutoScaleInterval:   time.Second,
	}

	assert := assert.New(t)

	ctx, cancelCtx := context.WithTimeout(context.Background(), 30*time.Second)

	wp := NewWP(NewLogger("worker_pool", "test"), newMockWorkerGen(taskDelay), wpConfig)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-wp.OutputCh:
			}
		}
	}()

	go wp.Run(ctx)
	defer wp.Stop()

	initialWorkerCount := int(wp.currWorkers.Load())
	assert.Equal(wpConfig.InitialWorkers, initialWorkerCount)

	for taskID := range taskCount {
		wp.AddTask(ctx, fmt.Sprintf("task-%d", taskID))
	}

	// Wait for auto scaling to kick in
	time.Sleep(3 * time.Second)

	scaledWorkerCount := int(wp.currWorkers.Load())
	assert.Greater(scaledWorkerCount, initialWorkerCount)

	cancelCtx()
}

func Test_WorkerPool_ScaleDown(t *testing.T) {
	taskDelay := 10 * time.Millisecond
	taskCount := 100

	wpConfig := &WorkerPoolConfig0{
		AutoScale:           true,
		InitialWorkers:      10,
		MinWorkers:          2,
		MaxWorkers:          10,
		QueueDepthPerWorker: 2,
		ScaleDownFactor:     0.1,
		AutoScaleInterval:   time.Second,
	}

	assert := assert.New(t)

	ctx, cancelCtx := context.WithTimeout(context.Background(), 30*time.Second)

	wp := NewWP(NewLogger("worker_pool", "test"), newMockWorkerGen(taskDelay), wpConfig)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-wp.OutputCh:
			}
		}
	}()

	go wp.Run(ctx)
	defer wp.Stop()

	for taskID := range taskCount {
		wp.AddTask(ctx, fmt.Sprintf("task-%d", taskID))
	}

	// Wait for auto scaling to kick in
	time.Sleep(3 * time.Second)

	scaledWorkerCount := int(wp.currWorkers.Load())
	assert.Less(scaledWorkerCount, wpConfig.InitialWorkers)

	cancelCtx()
}
