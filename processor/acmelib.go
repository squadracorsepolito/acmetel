package processor

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmetel/adapter"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
)

type AcmelibConfig struct {
	WorkerNum   int
	ChannelSize int
}

func NewDefaultAcmelibConfig() *AcmelibConfig {
	return &AcmelibConfig{
		WorkerNum:   1,
		ChannelSize: 8,
	}
}

type Acmelib struct {
	l     *internal.Logger
	stats *internal.Stats

	in connector.Connector[*adapter.CANMessageBatch]

	writerWg *sync.WaitGroup

	workerPool *internal.WorkerPool[*adapter.CANMessageBatch, int]
}

func NewAcmelib(cfg *AcmelibConfig) *Acmelib {
	l := internal.NewLogger("processor", "acmelib")

	return &Acmelib{
		l:     l,
		stats: internal.NewStats(l),

		writerWg: &sync.WaitGroup{},

		workerPool: internal.NewWorkerPool(cfg.WorkerNum, cfg.ChannelSize, newAcmelibWorkerGen(l)),
	}
}

func (a *Acmelib) Init(ctx context.Context) error {
	return nil
}

func (a *Acmelib) runWriter(ctx context.Context) {
	a.writerWg.Add(1)
	defer a.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.workerPool.OutputCh:
			// TODO! write to next stage
		}
	}
}

func (a *Acmelib) Run(ctx context.Context) {
	a.l.Info("running")

	received := 0
	skipped := 0
	defer func() {
		a.l.Info("received frames", "count", received)
		a.l.Info("skipped frames", "count", skipped)
	}()

	go a.stats.RunStats(ctx)

	go a.workerPool.Run(ctx)

	go a.runWriter(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		msgBatch, err := a.in.Read()
		if err != nil {
			a.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		a.stats.IncrementItemCount()

		received++

		select {
		case a.workerPool.InputCh <- msgBatch:
		default:
			skipped++
		}
	}
}

func (a *Acmelib) Stop() {
	defer a.l.Info("stopped")

	a.workerPool.Stop()
	a.writerWg.Wait()
}

func (a *Acmelib) SetInput(connector connector.Connector[*adapter.CANMessageBatch]) {
	a.in = connector
}

type acmelibWorker struct {
	*internal.BaseWorker
}

func newAcmelibWorkerGen(l *internal.Logger) internal.WorkerGen[*adapter.CANMessageBatch, int] {
	return func() internal.Worker[*adapter.CANMessageBatch, int] {
		return &acmelibWorker{
			BaseWorker: internal.NewBaseWorker(l),
		}
	}
}

func (w *acmelibWorker) DoWork(_ context.Context, msgBatch *adapter.CANMessageBatch) (int, error) {
	adapter.CANMessageBatchPoolInstance.Put(msgBatch)

	return 0, nil
}
