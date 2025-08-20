package message

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Base is the base struct that has to be embedded in all message types.
type Base struct {
	receiveTime time.Time
	timestamp   time.Time
	span        trace.SpanContext
}

// SetReceiveTime sets the time the message was received.
func (b *Base) SetReceiveTime(receiveTime time.Time) {
	b.receiveTime = receiveTime
}

// GetReceiveTime returns the time the message was received.
func (b *Base) GetReceiveTime() time.Time {
	return b.receiveTime
}

// SetTimestamp sets the timestamp of the message.
func (b *Base) SetTimestamp(timestamp time.Time) {
	b.timestamp = timestamp
}

// GetTimestamp returns the timestamp of the message.
// It may be different from the receive time.
func (b *Base) GetTimestamp() time.Time {
	return b.timestamp
}

// SaveSpan saves the trace span for the message.
func (b *Base) SaveSpan(span trace.Span) {
	b.span = span.SpanContext()
}

// LoadSpanContext loads the trace of the message
// into the provided context.
func (b *Base) LoadSpanContext(ctx context.Context) context.Context {
	return trace.ContextWithSpanContext(ctx, b.span)
}
