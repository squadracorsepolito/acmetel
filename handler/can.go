package handler

import (
	"context"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
	"go.opentelemetry.io/otel/attribute"
)

type CANConfig struct {
	*worker.PoolConfig

	Messages []*acmelib.Message
}

func NewDefaultCANConfig() *CANConfig {
	return &CANConfig{
		PoolConfig: worker.DefaultPoolConfig(),
	}
}

type CAN struct {
	*stage[*message.RawCANMessageBatch, *message.CANSignalBatch, *CANConfig, canWorker, *canDecoder, *canWorker]
}

func NewCAN(cfg *CANConfig) *CAN {
	return &CAN{
		stage: newStage[*message.RawCANMessageBatch, *message.CANSignalBatch, *CANConfig, canWorker, *canDecoder]("can", cfg),
	}
}

func (h *CAN) Init(ctx context.Context) error {
	decoder := newCANDecoder(h.cfg.Messages)

	return h.init(ctx, decoder)
}

func (h *CAN) Run(ctx context.Context) {
	h.run(ctx)
}

func (h *CAN) Stop() {
	h.close()
}

type canWorker struct {
	tel     *internal.Telemetry
	decoder *canDecoder
}

func (w *canWorker) Init(_ context.Context, decoder *canDecoder) error {
	w.decoder = decoder
	return nil
}

func (w *canWorker) Handle(ctx context.Context, msgBatch *message.RawCANMessageBatch) (*message.CANSignalBatch, error) {
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

func (w *canWorker) Stop(_ context.Context) error {
	return nil
}

func (w *canWorker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

type canDecoder struct {
	m map[uint32]func([]byte) []*acmelib.SignalDecoding
}

func newCANDecoder(messages []*acmelib.Message) *canDecoder {
	m := make(map[uint32]func([]byte) []*acmelib.SignalDecoding)

	for _, msg := range messages {
		m[uint32(msg.GetCANID())] = msg.SignalLayout().Decode
	}

	return &canDecoder{
		m: m,
	}
}

func (d *canDecoder) decode(ctx context.Context, canID uint32, data []byte) []*acmelib.SignalDecoding {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	fn, ok := d.m[canID]
	if !ok {
		return nil
	}
	return fn(data)
}
