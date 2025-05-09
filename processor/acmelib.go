package processor

import (
	"context"
	"sync"
	"time"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/adapter"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
)

type CANSignalBatch struct {
	Timestamp time.Time
	Signals   []CANSignal
}

type CANSignal struct {
	CANID     uint32
	Name      string
	RawValue  uint64
	ValueType acmelib.SignalValueType
	Value     any
	Unit      string
}

type AcmelibConfig struct {
	WorkerNum   int
	ChannelSize int
	Messages    []*acmelib.Message
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

	workerPool *internal.WorkerPool[*adapter.CANMessageBatch, *CANSignalBatch]
}

func NewAcmelib(cfg *AcmelibConfig) *Acmelib {
	l := internal.NewLogger("processor", "acmelib")
	decoder := newAcmelibDecoder(cfg.Messages)

	return &Acmelib{
		l:     l,
		stats: internal.NewStats(l),

		writerWg: &sync.WaitGroup{},

		workerPool: internal.NewWorkerPool(cfg.WorkerNum, cfg.ChannelSize, newAcmelibWorkerGen(l, decoder)),
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

	decoder *acmelibDecoder
}

func newAcmelibWorkerGen(l *internal.Logger, decoder *acmelibDecoder) internal.WorkerGen[*adapter.CANMessageBatch, *CANSignalBatch] {
	return func() internal.Worker[*adapter.CANMessageBatch, *CANSignalBatch] {
		return &acmelibWorker{
			BaseWorker: internal.NewBaseWorker(l),

			decoder: decoder,
		}
	}
}

func (w *acmelibWorker) DoWork(ctx context.Context, msgBatch *adapter.CANMessageBatch) (*CANSignalBatch, error) {
	adapter.CANMessageBatchPoolInstance.Put(msgBatch)

	sigBatch := &CANSignalBatch{
		Timestamp: msgBatch.Timestamp,
	}

	for i := range msgBatch.MessageCount {
		msg := msgBatch.Messages[i]

		decodings := w.decoder.decode(ctx, msg.CANID, msg.RawData)
		for _, dec := range decodings {
			sig := CANSignal{
				CANID:     msg.CANID,
				Name:      dec.Signal.Name(),
				RawValue:  dec.RawValue,
				ValueType: dec.ValueType,
				Value:     dec.Value,
				Unit:      dec.Unit,
			}

			sigBatch.Signals = append(sigBatch.Signals, sig)
		}
	}

	return sigBatch, nil
}

type acmelibDecoder struct {
	m map[uint32]func([]byte) []*acmelib.SignalDecoding
}

func newAcmelibDecoder(messages []*acmelib.Message) *acmelibDecoder {
	m := make(map[uint32]func([]byte) []*acmelib.SignalDecoding)

	for _, msg := range messages {
		m[uint32(msg.GetCANID())] = msg.SignalLayout().Decode
	}

	return &acmelibDecoder{
		m: m,
	}
}

func (ad *acmelibDecoder) decode(ctx context.Context, canID uint32, data []byte) []*acmelib.SignalDecoding {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	fn, ok := ad.m[canID]
	if !ok {
		return nil
	}
	return fn(data)
}
