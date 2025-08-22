package ticker

import (
	"context"
	"time"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

type Stage struct {
	*stage.Ingress[*Message]

	source *source

	interval time.Duration
}

func NewStage(outputConnector connector.Connector[*Message], cfg *Config) *Stage {
	source := newSource()

	return &Stage{
		Ingress: stage.NewIngress(
			"ticker", source, outputConnector, cfg.WriterQueueSize,
		),

		source: source,

		interval: cfg.Interval,
	}
}

func (s *Stage) Init(ctx context.Context) error {
	s.source.init(s.interval)

	return s.Ingress.Init(ctx)
}
