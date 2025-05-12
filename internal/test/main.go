package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
)

type mockWorker struct {
	delay time.Duration
}

func newMockWorkerGen(dalay time.Duration) internal.WorkerGen0[string, string] {
	return func() internal.Worker0[string, string] {
		return &mockWorker{delay: dalay}
	}
}

func (w *mockWorker) DoWork(ctx context.Context, dataIn string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
		time.Sleep(w.delay)
		return fmt.Sprintf("processed-%s", dataIn), nil
	}
}

func main() {
	ctx, cancelCtx := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer cancelCtx()

	wp := internal.NewWP(internal.NewLogger("worker_pool", "test"), newMockWorkerGen(100*time.Millisecond), internal.WorkerPoolConfigs{
		InitialWorkers:      2,
		MinWorkers:          2,
		MaxWorkers:          100,
		QueueDepthPerWorker: 20,
		AutoScaleInterval:   time.Second * 3,
		ScalingSensivity:    0.1, // 10%
	})

	go wp.Run(ctx)
	defer wp.Stop()

	go func() {
		i := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			wp.AddTask(ctx, fmt.Sprintf("task-%d", i))
			i++
			time.Sleep(time.Millisecond)
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-wp.OutputCh:
			}
		}
	}()

	<-ctx.Done()
}
