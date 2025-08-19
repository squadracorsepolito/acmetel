package internal

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type Message interface {
	SetReceiveTime(receiveTime time.Time)
	GetReceiveTime() time.Time
	SetTimestamp(timestamp time.Time)
	GetTimestamp() time.Time
	SaveSpan(span trace.Span)
	LoadSpanContext(ctx context.Context) context.Context
}

type ReOrderableMessage interface {
	Message

	GetSequenceNumber() uint64
}

type BaseMessage struct {
	receiveTime time.Time
	timestamp   time.Time
	span        trace.SpanContext
}

func NewBaseMessage(receiveTime time.Time, span trace.SpanContext) *BaseMessage {
	return &BaseMessage{
		receiveTime: receiveTime,
		span:        span,
	}
}

func (bm *BaseMessage) SetReceiveTime(receiveTime time.Time) {
	bm.receiveTime = receiveTime
}

func (bm *BaseMessage) SetTimestamp(timestamp time.Time) {
	bm.timestamp = timestamp
}

func (bm *BaseMessage) GetTimestamp() time.Time {
	return bm.timestamp
}

func (bm *BaseMessage) GetReceiveTime() time.Time {
	return bm.receiveTime
}

func (bm *BaseMessage) SaveSpan(span trace.Span) {
	bm.span = span.SpanContext()
}

func (bm *BaseMessage) LoadSpanContext(ctx context.Context) context.Context {
	return trace.ContextWithSpanContext(ctx, bm.span)
}

type RawDataMessage interface {
	Message

	GetRawData() []byte
}
