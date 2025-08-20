package pool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"go.opentelemetry.io/otel/metric"
)

// Egress is a worker pool intended to be used by an egress stage.
type Egress[In message.Message, W, InitArgs any, WPtr EgressWorkerPtr[W, InitArgs, In]] struct {
	tel *internal.Telemetry

	cfg *Config

	scaler *scaler

	initArgs InitArgs

	wg *sync.WaitGroup

	inputCh chan In

	deliveredMessages   atomic.Int64
	deliveringErrors    atomic.Int64
	messageTotHistogram *internal.Histogram
}

// NewEgress returns a new egress worker pool.
func NewEgress[In message.Message, W, InitArgs any, WPtr EgressWorkerPtr[W, InitArgs, In]](tel *internal.Telemetry, cfg *Config) *Egress[In, W, InitArgs, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8 * 32

	return &Egress[In, W, InitArgs, WPtr]{
		tel: tel,

		cfg: cfg,

		scaler: newScaler(tel, cfg.toScaler()),

		wg: &sync.WaitGroup{},

		inputCh: make(chan In, channelSize),
	}
}

// Init initialises the worker pool.
func (ep *Egress[In, W, InitArgs, WPtr]) Init(ctx context.Context, initArgs InitArgs) error {
	ep.initMetrics()

	ep.initArgs = initArgs
	ep.scaler.init(ctx, ep.cfg.InitialWorkers)

	return nil
}

func (ep *Egress[In, W, InitArgs, WPtr]) initMetrics() {
	ep.tel.NewCounter("worker_pool_delivered_messages", func() int64 { return ep.deliveredMessages.Load() })
	ep.tel.NewCounter("worker_pool_delivering_errors", func() int64 { return ep.deliveringErrors.Load() })

	ep.messageTotHistogram = ep.tel.NewHistogram("total_message_processing_time", metric.WithUnit("ms"))
}

// Run runs the worker pool.
func (ep *Egress[In, W, InitArgs, WPtr]) Run(ctx context.Context) {
	go ep.runStartWorkerListener(ctx)
	go ep.scaler.run(ctx)
}

func (ep *Egress[In, W, InitArgs, WPtr]) runStartWorkerListener(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-ep.scaler.getStartCh():
			go ep.runWorker(ctx)
		}
	}
}

func (ep *Egress[In, W, InitArgs, WPtr]) runWorker(ctx context.Context) {
	var dummyWorker W
	worker := WPtr(&dummyWorker)

	worker.SetTelemetry(ep.tel)

	if err := worker.Init(ctx, ep.initArgs); err != nil {
		ep.tel.LogError("failed to init worker", err)
		return
	}

	ep.wg.Add(1)
	defer ep.wg.Done()

	workerID := ep.scaler.notifyWorkerStart()
	defer ep.scaler.notifyWorkerStop()

	ep.tel.LogInfo("starting worker", "worker_id", workerID)

	defer func() {
		ep.tel.LogInfo("stopping worker", "worker_id", workerID)

		if err := worker.Close(ctx); err != nil {
			ep.tel.LogError("failed to stop worker", err, "worker_id", workerID)
		}
	}()

	stopCh := ep.scaler.getStopCh(workerID)

	for {
		select {
		case <-ctx.Done():
			return

		case <-stopCh:
			return

		case msgIn := <-ep.inputCh:
			if err := worker.Deliver(ctx, msgIn); err != nil {
				ep.tel.LogError("failed to deliver message", err, "worker_id", workerID)
				ep.deliveringErrors.Add(1)

				goto loopCleanup
			}

			ep.deliveredMessages.Add(1)

			ep.messageTotHistogram.Record(ctx, time.Since(msgIn.GetReceiveTime()).Milliseconds())

		loopCleanup:
			ep.scaler.notifyTaskCompleted()
		}
	}
}

// Close closes the worker pool.
func (ep *Egress[In, W, InitArgs, WPtr]) Close() {
	ep.tel.LogInfo("stopping worker pool")

	ep.wg.Wait()
	ep.scaler.stop()

	close(ep.inputCh)
}

// AddTask adds a new task to the worker pool.
func (ep *Egress[In, W, InitArgs, WPtr]) AddTask(ctx context.Context, task In) bool {
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
