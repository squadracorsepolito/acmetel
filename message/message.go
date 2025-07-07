package message

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
)

type Message interface {
	SetReceiveTime(receiveTime time.Time)
	ReceiveTime() time.Time
	SaveSpan(span trace.Span)
	LoadSpanContext(ctx context.Context) context.Context
}

type embedded struct {
	receiveTime time.Time
	span        trace.SpanContext
}

func (e *embedded) SetReceiveTime(receiveTime time.Time) {
	e.receiveTime = receiveTime
}

func (e *embedded) ReceiveTime() time.Time {
	return e.receiveTime
}

func (e *embedded) SaveSpan(span trace.Span) {
	e.span = span.SpanContext()
}

func (e *embedded) LoadSpanContext(ctx context.Context) context.Context {
	return trace.ContextWithSpanContext(ctx, e.span)
}

type ReOrderableMessage interface {
	Message
	SequenceNumber() uint64
	LogicalTime() time.Time
	SetAdjustedTime(adjustedTime time.Time)
}
