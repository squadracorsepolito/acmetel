package can

import (
	"context"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/internal"
	"go.opentelemetry.io/otel/attribute"
)

type workerArgs struct {
	decoder *decoder
}

type worker[T msgIn] struct {
	tel *internal.Telemetry

	decoder *decoder
}

func (w *worker[T]) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *worker[T]) Init(_ context.Context, workerArgs *workerArgs) error {
	w.decoder = workerArgs.decoder
	return nil
}

func (w *worker[T]) Handle(ctx context.Context, msgIn T) (*Message, error) {
	ctx, span := w.tel.NewTrace(ctx, "process raw CAN message batch")
	defer span.End()

	res := newMessage()
	res.SetTimestamp(msgIn.GetTimestamp())

	for _, msg := range msgIn.GetRawCANMessages() {
		canID := msg.CANID

		decodings := w.decoder.decode(ctx, canID, msg.RawData)
		for _, dec := range decodings {
			sig := CANSignal{
				CANID:    int64(canID),
				Name:     dec.Signal.Name(),
				RawValue: dec.RawValue,
			}

			switch dec.ValueType {
			case acmelib.SignalValueTypeFlag:
				sig.Type = ValueTypeFlag
				sig.ValueFlag = dec.ValueAsFlag()

			case acmelib.SignalValueTypeInt:
				sig.Type = ValueTypeInt
				sig.ValueInt = dec.ValueAsInt()

			case acmelib.SignalValueTypeUint:
				sig.Type = ValueTypeInt
				sig.ValueInt = int64(dec.ValueAsUint())

			case acmelib.SignalValueTypeFloat:
				sig.Type = ValueTypeFloat
				sig.ValueFloat = dec.ValueAsFloat()

			case acmelib.SignalValueTypeEnum:
				sig.Type = ValueTypeEnum
				sig.ValueEnum = dec.ValueAsEnum()
			}

			res.Signals = append(res.Signals, sig)
			res.SignalCount++
		}
	}

	span.SetAttributes(attribute.Int("message_count", len(msgIn.GetRawCANMessages())))

	return res, nil
}

func (w *worker[T]) Close(_ context.Context) error {
	return nil
}
