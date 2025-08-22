package ticker

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/stage"
	"go.opentelemetry.io/otel/attribute"
)

var _ stage.Source[*Message] = (*source)(nil)

type source struct {
	tel *internal.Telemetry

	ticker *time.Ticker

	// Telemetry metrics
	triggeredMessages atomic.Int64
}

func newSource() *source {
	return &source{}
}

func (s *source) SetTelemetry(tel *internal.Telemetry) {
	s.tel = tel
}

func (s *source) init(interval time.Duration) {
	s.ticker = time.NewTicker(interval)
}

func (s *source) Run(ctx context.Context, out chan<- *Message) {
	triggerCount := 0

	for {
		triggerCount++

		select {
		case <-ctx.Done():
			return
		case <-s.ticker.C:
			out <- s.handleTrigger(ctx, triggerCount)
		}
	}
}

func (s *source) handleTrigger(ctx context.Context, count int) *Message {
	_, span := s.tel.NewTrace(ctx, "triggered ticker message")
	defer span.End()

	msg := newMessage()

	triggerTime := time.Now()
	msg.SetReceiveTime(triggerTime)
	msg.SetTimestamp(triggerTime)

	msg.TriggerNumber = count

	span.SetAttributes(attribute.Int("trigger_number", count))
	msg.SaveSpan(span)

	return msg
}
