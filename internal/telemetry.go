package internal

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type Telemetry struct {
	stageKind string
	stageName string

	l *Logger

	tracer trace.Tracer
	meter  metric.Meter
}

func NewTelemetry(stageKind, stageName string) *Telemetry {
	return &Telemetry{
		stageKind: stageKind,
		stageName: stageName,

		l: NewLogger(stageKind, stageName),

		tracer: otel.GetTracerProvider().Tracer("acmetel"),
		meter:  otel.GetMeterProvider().Meter("acmetel"),
	}
}

func (t *Telemetry) Logger() *Logger {
	return t.l
}

func (t *Telemetry) LogInfo(msg string, args ...any) {
	t.l.Info(msg, args...)
}

func (t *Telemetry) LogWarn(msg string, args ...any) {
	t.l.Warn(msg, args...)
}

func (t *Telemetry) LogError(msg string, err error, args ...any) {
	t.l.Error(msg, err, args...)
}

func (t *Telemetry) setDefaultAttributes(span trace.Span) {
	span.SetAttributes(
		attribute.String("acmetel.stage_kind", t.stageKind),
		attribute.String("acmetel.stage_name", t.stageName),
	)
}

func (t *Telemetry) NewTrace(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	ctx, span := t.tracer.Start(ctx, spanName, opts...)
	t.setDefaultAttributes(span)
	return ctx, span
}

func (t *Telemetry) getMeterName(name string) string {
	return fmt.Sprintf("%s_%s_%s", t.stageKind, t.stageName, name)
}

func (t *Telemetry) NewCounter(name string, opts ...metric.Int64CounterOption) metric.Int64Counter {
	counterName := t.getMeterName(name)
	counter, err := t.meter.Int64Counter(counterName, opts...)
	if err != nil {
		t.LogError("failed to create counter", err, "name", name)
	}

	t.LogInfo("created counter", "name", counterName)

	return counter
}

func (t *Telemetry) NewUpDownCounter(name string, opts ...metric.Int64UpDownCounterOption) metric.Int64UpDownCounter {
	counterName := t.getMeterName(name)
	counter, err := t.meter.Int64UpDownCounter(counterName, opts...)
	if err != nil {
		t.LogError("failed to create up/down counter", err, "name", name)
	}

	t.LogInfo("created up/down counter", "name", counterName)

	return counter
}
