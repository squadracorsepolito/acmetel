// Package message contains the interfaces for the various message types
// and common implementations.
package message

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Message interface defines the common methods for all message types.
type Message interface {
	// SetReceiveTime sets the time the message was received.
	SetReceiveTime(receiveTime time.Time)
	// GetReceiveTime returns the time the message was received.
	GetReceiveTime() time.Time

	// SetTimestamp sets the timestamp of the message.
	SetTimestamp(timestamp time.Time)
	// GetTimestamp returns the timestamp of the message.
	// It may be different from the receive time.
	GetTimestamp() time.Time

	// SaveSpan saves the trace span for the message.
	SaveSpan(span trace.Span)
	// LoadSpanContext loads the trace of the message
	// into the provided context.
	LoadSpanContext(ctx context.Context) context.Context
}

// ReOrderable interface defines the common methods for all re-orderable message types.
// This is used in the context of the re-order buffer.
type ReOrderable interface {
	Message

	// GetSequenceNumber returns the sequence number of the message.
	GetSequenceNumber() uint64
}

// Serializable interface defines the common methods for all message types
// that can be serialized into bytes.
// It is intended to be used by stages that need to process a stream of bytes.
type Serializable interface {
	Message

	// GetBytes returns the bytes of the message.
	GetBytes() []byte
}
