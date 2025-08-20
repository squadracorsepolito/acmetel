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

type worker[T RawCANMessageCarrier] struct {
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
	// Extract the span context from the input message
	ctx, span := w.tel.NewTrace(msgIn.LoadSpanContext(ctx), "handle CAN message batch")
	defer span.End()

	// Create the CAN message
	canMsg := newMessage()

	canMsg.SetReceiveTime(msgIn.GetReceiveTime())
	canMsg.SetTimestamp(msgIn.GetTimestamp())

	for _, msg := range msgIn.GetRawMessages() {
		canID := msg.CANID

		decodings := w.decoder.decode(ctx, canID, msg.RawData)
		for _, dec := range decodings {
			sig := Signal{
				CANID:    canID,
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

			canMsg.Signals = append(canMsg.Signals, sig)
			canMsg.SignalCount++
		}
	}

	// Save the span in the message
	span.SetAttributes(attribute.Int("signal_count", canMsg.SignalCount))
	canMsg.SaveSpan(span)

	return canMsg, nil
}

func (w *worker[T]) Close(_ context.Context) error {
	return nil
}
