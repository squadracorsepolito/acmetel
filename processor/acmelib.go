package processor

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/adapter"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/egress"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/worker"
)

type AcmelibConfig struct {
	*worker.PoolConfig

	Messages []*acmelib.Message
}

func NewDefaultAcmelibConfig() *AcmelibConfig {
	return &AcmelibConfig{
		PoolConfig: worker.DefaultPoolConfig(),
	}
}

type Acmelib struct {
	l     *internal.Logger
	stats *internal.Stats

	cfg *AcmelibConfig

	in  connector.Connector[*adapter.CANMessageBatch]
	out connector.Connector[*egress.CANSignalBatch]

	writerWg *sync.WaitGroup

	workerPool *acmelibWorkerPool
}

func NewAcmelib(cfg *AcmelibConfig) *Acmelib {
	l := internal.NewLogger("processor", "acmelib")

	return &Acmelib{
		l:     l,
		stats: internal.NewStats(l),

		cfg: cfg,

		writerWg: &sync.WaitGroup{},

		workerPool: worker.NewPool[acmelibWorker, *acmelibDecoder, *adapter.CANMessageBatch, *egress.CANSignalBatch](l, cfg.PoolConfig),
	}
}

func (a *Acmelib) Init(ctx context.Context) error {
	decoder := newAcmelibDecoder(a.cfg.Messages)
	a.workerPool.Init(ctx, decoder)

	return nil
}

func (a *Acmelib) runWriter(ctx context.Context) {
	a.writerWg.Add(1)
	defer a.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case data := <-a.workerPool.GetOutputCh():
			if err := a.out.Write(data); err != nil {
				a.l.Warn("failed to write into output connector", "reason", err)
			}
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

		if !a.workerPool.AddTask(ctx, msgBatch) {
			skipped++
		}
	}
}

func (a *Acmelib) Stop() {
	defer a.l.Info("stopped")

	a.out.Close()
	a.workerPool.Stop()
	a.writerWg.Wait()
}

func (a *Acmelib) SetInput(connector connector.Connector[*adapter.CANMessageBatch]) {
	a.in = connector
}

func (a *Acmelib) SetOutput(connector connector.Connector[*egress.CANSignalBatch]) {
	a.out = connector
}

type acmelibWorkerPool = worker.Pool[acmelibWorker, *acmelibDecoder, *adapter.CANMessageBatch, *egress.CANSignalBatch, *acmelibWorker]

type acmelibWorker struct {
	decoder *acmelibDecoder
}

func (w *acmelibWorker) Init(_ context.Context, decoder *acmelibDecoder) error {
	w.decoder = decoder
	return nil
}

func (w *acmelibWorker) DoWork(ctx context.Context, msgBatch *adapter.CANMessageBatch) (*egress.CANSignalBatch, error) {
	ctx, span := internal.Tracer.Start(ctx, "acmelib decoding")
	defer span.End()

	adapter.CANMessageBatchPoolInstance.Put(msgBatch)

	sigBatch := &egress.CANSignalBatch{
		Timestamp: msgBatch.Timestamp,
	}

	for i := range msgBatch.MessageCount {
		msg := msgBatch.Messages[i]

		decodings := w.decoder.decode(ctx, msg.CANID, msg.RawData)
		for _, dec := range decodings {
			sig := egress.CANSignal{
				CANID:    int64(msg.CANID),
				Name:     dec.Signal.Name(),
				RawValue: int64(dec.RawValue),
			}

			switch dec.ValueType {
			case acmelib.SignalValueTypeFlag:
				sig.Table = egress.CANSignalTableFlag
				sig.ValueFlag = dec.ValueAsFlag()

			case acmelib.SignalValueTypeInt:
				sig.Table = egress.CANSignalTableInt
				sig.ValueInt = dec.ValueAsInt()

			case acmelib.SignalValueTypeUint:
				sig.Table = egress.CANSignalTableInt
				sig.ValueInt = int64(dec.ValueAsUint())

			case acmelib.SignalValueTypeFloat:
				sig.Table = egress.CANSignalTableFloat
				sig.ValueFloat = dec.ValueAsFloat()

			case acmelib.SignalValueTypeEnum:
				sig.Table = egress.CANSignalTableEnum
				sig.ValueEnum = dec.ValueAsEnum()
			}

			sigBatch.Signals = append(sigBatch.Signals, sig)
			sigBatch.SignalCount++
		}
	}

	return sigBatch, nil
}

func (w *acmelibWorker) Stop(_ context.Context) error {
	return nil
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
