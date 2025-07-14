package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
	"go.opentelemetry.io/otel/metric"
)

type EgressPool[In internal.Message, W, InitArgs any, WPtr EgressWorkerPtr[W, InitArgs, In]] struct {
	tel *internal.Telemetry

	cfg *PoolConfig

	scaler *scaler

	initArgs InitArgs

	wg *sync.WaitGroup

	inputCh chan In

	deliveredMessages   atomic.Int64
	deliveringErrors    atomic.Int64
	messageTotHistogram *internal.Histogram
}

func NewEgressPool[In internal.Message, W, InitArgs any, WPtr EgressWorkerPtr[W, InitArgs, In]](tel *internal.Telemetry, cfg *PoolConfig) *EgressPool[In, W, InitArgs, WPtr] {
	channelSize := cfg.MaxWorkers * cfg.QueueDepthPerWorker * 8 * 32

	return &EgressPool[In, W, InitArgs, WPtr]{
		tel: tel,

		cfg: cfg,

		scaler: newScaler(tel, cfg.toScaler()),

		wg: &sync.WaitGroup{},

		inputCh: make(chan In, channelSize),
	}
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) Init(ctx context.Context, initArgs InitArgs) error {
	ep.initMetrics()

	ep.initArgs = initArgs
	ep.scaler.init(ctx, ep.cfg.InitialWorkers)

	return nil
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) initMetrics() {
	ep.tel.NewCounter("worker_pool_delivered_messages", func() int64 { return ep.deliveredMessages.Load() })
	ep.tel.NewCounter("worker_pool_delivering_errors", func() int64 { return ep.deliveringErrors.Load() })

	ep.messageTotHistogram = ep.tel.NewHistogram("total_message_processing_time", metric.WithUnit("ms"))
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) Run(ctx context.Context) {
	go ep.runStartWorkerListener(ctx)
	go ep.scaler.run(ctx)
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) runStartWorkerListener(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-ep.scaler.getStartCh():
			go ep.runWorker(ctx)
		}
	}
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) runWorker(ctx context.Context) {
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

		if err := worker.Stop(ctx); err != nil {
			ep.tel.LogError("failed to stop worker", err, "worker_id", workerID)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ep.scaler.getStopCh(workerID):
			return

		case msgIn := <-ep.inputCh:
			tracedCtx, span := ep.tel.NewTrace(msgIn.LoadSpanContext(ctx), "deliver message")

			receiveTime := msgIn.GetTimestamp()

			if err := worker.Deliver(tracedCtx, msgIn); err != nil {
				ep.tel.LogError("failed to deliver message", err, "worker_id", workerID)
				ep.deliveringErrors.Add(1)

				goto loopCleanup
			}

			ep.deliveredMessages.Add(1)

			ep.messageTotHistogram.Record(tracedCtx, time.Since(receiveTime).Milliseconds())

		loopCleanup:
			ep.scaler.notifyTaskCompleted()
			span.End()
		}
	}
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) Stop() {
	ep.tel.LogInfo("stopping worker pool")

	ep.wg.Wait()
	ep.scaler.stop()

	close(ep.inputCh)
}

func (ep *EgressPool[In, W, InitArgs, WPtr]) AddTask(ctx context.Context, task In) bool {
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
