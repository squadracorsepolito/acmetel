package internal

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type Message interface {
	SetReceiveTime(receiveTime time.Time)
	GetReceiveTime() time.Time
	GetTimestamp() time.Time
	SaveSpan(span trace.Span)
	LoadSpanContext(ctx context.Context) context.Context
}

type ReOrderableMessage interface {
	Message

	GetSequenceNumber() uint64
	LogicalTime() time.Time
	SetAdjustedTime(adjustedTime time.Time)
}

type BaseMessage struct {
	receiveTime time.Time
	span        trace.SpanContext
}

func (bm *BaseMessage) SetReceiveTime(receiveTime time.Time) {
	bm.receiveTime = receiveTime
}

func (bm *BaseMessage) GetTimestamp() time.Time {
	return bm.receiveTime
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
