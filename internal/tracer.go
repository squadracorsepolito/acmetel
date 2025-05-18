package internal

import (
	"go.opentelemetry.io/otel/trace"
)

var Tracer trace.Tracer

func SetTracer(tracer trace.Tracer) {
	Tracer = tracer
}
