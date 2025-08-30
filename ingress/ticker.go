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

// TickerConfig structs contains the configuration for the Ticker stage.
type TickerConfig struct {
	// Interval is the duration between ticks.
	//
	// Default: 100ms
	Interval time.Duration `yaml:"interval" json:"interval"`
}

// DefaultTickerConfig returns the default configuration for the Ticker stage.
func DefaultTickerConfig() *TickerConfig {
	return &TickerConfig{
		Interval: 100 * time.Millisecond,
	}
}

///////////////
//  MESSAGE  //
///////////////

// TickerMessage is the message returned by the Ticker stage.
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

	// Metrics
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

func (ts *tickerSource) Run(ctx context.Context, outConnector conn[*TickerMessage]) {
	ticks := 0

	for {
		ticks++

		select {
		case <-ctx.Done():
			return
		case <-ts.ticker.C:
			if err := outConnector.Write(ts.handleTrigger(ctx, ticks)); err != nil {
				ts.tel.LogError("failed to write message to output connector", err)
			}
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

// TickerStage is an ingress stage that ticks periodically.
type TickerStage struct {
	*stage.Ingress[*TickerMessage]

	cfg *TickerConfig

	source *tickerSource
}

// NewTickerStage returns a new Ticker stage.
func NewTickerStage(outConnector connector.Connector[*TickerMessage], cfg *TickerConfig) *TickerStage {
	source := newTickerSource()

	return &TickerStage{
		Ingress: stage.NewIngress("ticker", source, outConnector),

		cfg: cfg,

		source: source,
	}
}

// Init initializes the stage.
func (s *TickerStage) Init(ctx context.Context) error {
	s.source.init(s.cfg.Interval)

	return s.Ingress.Init(ctx)
}
