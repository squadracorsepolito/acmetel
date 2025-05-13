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

	configs := internal.NewDefaultWorkerPoolConfig()
	configs.MaxWorkers = 64
	wp := internal.NewWP(internal.NewLogger("worker_pool", "test"), newMockWorkerGen(100*time.Millisecond), configs)

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
