package processor

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
	"go.opentelemetry.io/otel/attribute"
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
	l   *internal.Logger
	tel *internal.Telemetry

	stats *internal.Stats

	cfg *AcmelibConfig

	in  connector.Connector[*message.RawCANMessageBatch]
	out connector.Connector[*message.CANSignalBatch]

	writerWg *sync.WaitGroup

	workerPool *acmelibWorkerPool
}

func NewAcmelib(cfg *AcmelibConfig) *Acmelib {
	tel := internal.NewTelemetry("processor", "acmelib")
	l := tel.Logger()

	return &Acmelib{
		l:   l,
		tel: tel,

		stats: internal.NewStats(l),

		cfg: cfg,

		writerWg: &sync.WaitGroup{},

		workerPool: worker.NewPool[acmelibWorker, *acmelibDecoder, *message.RawCANMessageBatch, *message.CANSignalBatch](tel, cfg.PoolConfig),
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

func (a *Acmelib) SetInput(connector connector.Connector[*message.RawCANMessageBatch]) {
	a.in = connector
}

func (a *Acmelib) SetOutput(connector connector.Connector[*message.CANSignalBatch]) {
	a.out = connector
}

type acmelibWorkerPool = worker.Pool[acmelibWorker, *acmelibDecoder, *message.RawCANMessageBatch, *message.CANSignalBatch, *acmelibWorker]

type acmelibWorker struct {
	tel     *internal.Telemetry
	decoder *acmelibDecoder
}

func (w *acmelibWorker) Init(_ context.Context, decoder *acmelibDecoder) error {
	w.decoder = decoder
	return nil
}

func (w *acmelibWorker) Handle(ctx context.Context, msgBatch *message.RawCANMessageBatch) (*message.CANSignalBatch, error) {
	defer message.PutRawCANMessageBatch(msgBatch)

	ctx, span := w.tel.NewTrace(ctx, "process raw CAN message batch")
	defer span.End()

	sigBatch := message.NewCANSignalBatch()
	sigBatch.Timestamp = msgBatch.Timestamp

	for i := range msgBatch.MessageCount {
		msg := msgBatch.Messages[i]

		decodings := w.decoder.decode(ctx, msg.CANID, msg.RawData)
		for _, dec := range decodings {
			sig := message.CANSignal{
				CANID:    int64(msg.CANID),
				Name:     dec.Signal.Name(),
				RawValue: int64(dec.RawValue),
			}

			switch dec.ValueType {
			case acmelib.SignalValueTypeFlag:
				sig.Table = message.CANSignalTableFlag
				sig.ValueFlag = dec.ValueAsFlag()

			case acmelib.SignalValueTypeInt:
				sig.Table = message.CANSignalTableInt
				sig.ValueInt = dec.ValueAsInt()

			case acmelib.SignalValueTypeUint:
				sig.Table = message.CANSignalTableInt
				sig.ValueInt = int64(dec.ValueAsUint())

			case acmelib.SignalValueTypeFloat:
				sig.Table = message.CANSignalTableFloat
				sig.ValueFloat = dec.ValueAsFloat()

			case acmelib.SignalValueTypeEnum:
				sig.Table = message.CANSignalTableEnum
				sig.ValueEnum = dec.ValueAsEnum()
			}

			sigBatch.Signals = append(sigBatch.Signals, sig)
			sigBatch.SignalCount++
		}
	}

	span.SetAttributes(attribute.Int("message_count", msgBatch.MessageCount))

	return sigBatch, nil
}

func (w *acmelibWorker) Stop(_ context.Context) error {
	return nil
}

func (w *acmelibWorker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
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
