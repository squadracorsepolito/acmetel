package processor

import (
	"context"
	"sync/atomic"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/stage"
	"go.opentelemetry.io/otel/attribute"
)

//////////////
//  CONFIG  //
//////////////

type CANConfig struct {
	PoolConfig *pool.Config

	Messages []*acmelib.Message
}

func DefaultCANConfig() *CANConfig {
	return &CANConfig{
		PoolConfig: pool.DefaultConfig(),

		Messages: []*acmelib.Message{},
	}
}

///////////////
//  MESSAGE  //
///////////////

// CANMessageCarrier interface defines the common methods
// for all message types that carry CAN messages.
type CANMessageCarrier interface {
	message.Message

	// GetRawMessages returns the list of raw CAN messages.
	GetRawMessages() []CANRawMessage
}

// CANRawMessage represents a CAN message before decoding.
type CANRawMessage struct {
	// CANID is the CAN ID of the message.
	CANID uint32

	// RawData is the payload of the CAN message.
	RawData []byte
	// DataLen is the number of bytes of the payload.
	DataLen int
}

// CANSignalValueType represents the type of the value of a signal.
type CANSignalValueType int

const (
	// CANSignalValueTypeFlag defines a value of type flag (boolean).
	CANSignalValueTypeFlag CANSignalValueType = iota
	// CANSignalValueTypeInt defines a value of type integer.
	CANSignalValueTypeInt
	// CANSignalValueTypeFloat defines a value of type float.
	CANSignalValueTypeFloat
	// CANSignalValueTypeEnum defines a value of type enum.
	CANSignalValueTypeEnum
)

// CANSignal represents a decoded CAN signal.
type CANSignal struct {
	// CANID is the CAN ID of the message that contains this signal.
	CANID uint32

	// Name is the name of the signal.
	Name string

	// RawValue is the raw value of the signal.
	RawValue uint64

	// Type is the type of the value of the signal.
	Type CANSignalValueType
	// ValueFlag is the value of the signal as a boolean.
	ValueFlag bool
	// ValueInt is the value of the signal as an integer.
	ValueInt int64
	// ValueFloat is the value of the signal as a float.
	ValueFloat float64
	// ValueEnum is the value of the signal as an enum.
	ValueEnum string
}

// CANMessage represents a decoded CAN message.
// It only contains the value of the signals of every message.
type CANMessage struct {
	message.Base

	// Signals is the list of decoded signals.
	Signals []CANSignal
	// SignalCount is the number of decoded signals.
	SignalCount int
}

func newCANMessage() *CANMessage {
	return &CANMessage{
		SignalCount: 0,
		Signals:     []CANSignal{},
	}
}

///////////////
//  DECODER  //
///////////////

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

func (cd *canDecoder) decode(ctx context.Context, canID uint32, data []byte) []*acmelib.SignalDecoding {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	fn, ok := cd.m[canID]
	if !ok {
		return nil
	}
	return fn(data)
}

//////////////
//  WORKER  //
//////////////

type canWorkerArgs struct {
	decoder *canDecoder
}

func newCANWorkerArgs(decoder *canDecoder) *canWorkerArgs {
	return &canWorkerArgs{
		decoder: decoder,
	}
}

type canWorker[T CANMessageCarrier] struct {
	tel *internal.Telemetry

	decoder *canDecoder

	// Metrics
	canMessages atomic.Int64
	canSignals  atomic.Int64
}

func (cw *canWorker[T]) SetTelemetry(tel *internal.Telemetry) {
	cw.tel = tel
}

func (cw *canWorker[T]) Init(_ context.Context, args *canWorkerArgs) error {
	cw.decoder = args.decoder

	cw.initMetrics()

	return nil
}

func (cw *canWorker[T]) initMetrics() {
	cw.tel.NewCounter("can_messages", func() int64 { return cw.canMessages.Load() })
	cw.tel.NewCounter("can_signals", func() int64 { return cw.canSignals.Load() })
}

func (cw *canWorker[T]) Handle(ctx context.Context, msgIn T) (*CANMessage, error) {
	// Extract the span context from the input message
	ctx, span := cw.tel.NewTrace(msgIn.LoadSpanContext(ctx), "handle CAN message batch")
	defer span.End()

	// Create the CAN message
	canMsg := newCANMessage()

	rawMessages := msgIn.GetRawMessages()
	rawMsgCount := len(rawMessages)

	for _, msg := range rawMessages {
		canID := msg.CANID

		decodings := cw.decoder.decode(ctx, canID, msg.RawData)
		for _, dec := range decodings {
			sig := CANSignal{
				CANID:    canID,
				Name:     dec.Signal.Name(),
				RawValue: dec.RawValue,
			}

			switch dec.ValueType {
			case acmelib.SignalValueTypeFlag:
				sig.Type = CANSignalValueTypeFlag
				sig.ValueFlag = dec.ValueAsFlag()

			case acmelib.SignalValueTypeInt:
				sig.Type = CANSignalValueTypeInt
				sig.ValueInt = dec.ValueAsInt()

			case acmelib.SignalValueTypeUint:
				sig.Type = CANSignalValueTypeInt
				sig.ValueInt = int64(dec.ValueAsUint())

			case acmelib.SignalValueTypeFloat:
				sig.Type = CANSignalValueTypeFloat
				sig.ValueFloat = dec.ValueAsFloat()

			case acmelib.SignalValueTypeEnum:
				sig.Type = CANSignalValueTypeEnum
				sig.ValueEnum = dec.ValueAsEnum()
			}

			canMsg.Signals = append(canMsg.Signals, sig)
			canMsg.SignalCount++
		}
	}

	// Save the span in the message
	span.SetAttributes(attribute.Int("signal_count", canMsg.SignalCount))
	canMsg.SaveSpan(span)

	// Update metrics
	cw.canMessages.Add(int64(rawMsgCount))
	cw.canSignals.Add(int64(canMsg.SignalCount))

	return canMsg, nil
}

func (cw *canWorker[T]) Close(_ context.Context) error {
	return nil
}

/////////////
//  STAGE  //
/////////////

type CANStage[T CANMessageCarrier] struct {
	*stage.Processor[T, *CANMessage, canWorker[T], *canWorkerArgs, *canWorker[T]]

	cfg *CANConfig
}

func NewCANStage[T CANMessageCarrier](inputConnector conn[T], outputConnector conn[*CANMessage], cfg *CANConfig) *CANStage[T] {
	return &CANStage[T]{
		Processor: stage.NewProcessor[T, *CANMessage, canWorker[T], *canWorkerArgs]("can", inputConnector, outputConnector, cfg.PoolConfig),

		cfg: cfg,
	}
}

func (cs *CANStage[T]) Init(ctx context.Context) error {
	decoder := newCANDecoder(cs.cfg.Messages)

	return cs.Processor.Init(ctx, newCANWorkerArgs(decoder))
}
