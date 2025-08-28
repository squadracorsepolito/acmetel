package ingress

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/stage"
	"go.opentelemetry.io/otel/attribute"
)

//////////////
//  CONFIG  //
//////////////

type TickerConfig struct {
	WriterQueueSize int

	Interval time.Duration
}

func DefaultTickerConfig() *TickerConfig {
	return &TickerConfig{
		WriterQueueSize: 256,

		Interval: 100 * time.Millisecond,
	}
}

///////////////
//  MESSAGE  //
///////////////

type TickerMessage struct {
	message.Base

	TickNumber int
}

func newTickerMessage() *TickerMessage {
	return &TickerMessage{}
}

//////////////
//  SOURCE  //
//////////////

var _ stage.Source[*TickerMessage] = (*tickerSource)(nil)

type tickerSource struct {
	tel *internal.Telemetry

	ticker *time.Ticker

	// Telemetry metrics
	triggeredMessages atomic.Int64
}

func newTickerSource() *tickerSource {
	return &tickerSource{}
}

func (ts *tickerSource) SetTelemetry(tel *internal.Telemetry) {
	ts.tel = tel
}

func (ts *tickerSource) init(interval time.Duration) {
	ts.ticker = time.NewTicker(interval)
}

func (ts *tickerSource) Run(ctx context.Context, out chan<- *TickerMessage) {
	ticks := 0

	for {
		ticks++

		select {
		case <-ctx.Done():
			return
		case <-ts.ticker.C:
			out <- ts.handleTrigger(ctx, ticks)
		}
	}
}

func (ts *tickerSource) handleTrigger(ctx context.Context, tick int) *TickerMessage {
	_, span := ts.tel.NewTrace(ctx, "triggered ticker message")
	defer span.End()

	msg := newTickerMessage()

	triggerTime := time.Now()
	msg.SetReceiveTime(triggerTime)
	msg.SetTimestamp(triggerTime)

	msg.TickNumber = tick

	span.SetAttributes(attribute.Int("tick_number", tick))
	msg.SaveSpan(span)

	return msg
}

/////////////
//  STAGE  //
/////////////

type TickerStage struct {
	*stage.Ingress[*TickerMessage]

	cfg *TickerConfig

	source *tickerSource
}

func NewTickerStage(outConnector connector.Connector[*TickerMessage], cfg *TickerConfig) *TickerStage {
	source := newTickerSource()

	return &TickerStage{
		Ingress: stage.NewIngress(
			"ticker", source, outConnector, cfg.WriterQueueSize,
		),

		cfg: cfg,

		source: source,
	}
}

func (s *TickerStage) Init(ctx context.Context) error {
	s.source.init(s.cfg.Interval)

	return s.Ingress.Init(ctx)
}
