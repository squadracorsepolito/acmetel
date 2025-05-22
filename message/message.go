package message

import (
	"context"

	"go.opentelemetry.io/otel/trace"
)

type Traceable interface {
	SaveSpan(span trace.Span)
	LoadSpanContext(ctx context.Context) context.Context
}

type embedded struct {
	Span trace.SpanContext
}

func (e *embedded) SaveSpan(span trace.Span) {
	e.Span = span.SpanContext()
}

func (e *embedded) LoadSpanContext(ctx context.Context) context.Context {
	return trace.ContextWithSpanContext(ctx, e.Span)
}
