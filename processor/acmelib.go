package processor

import (
	"context"

	"github.com/squadracorsepolito/acmetel/adapter"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
)

type Processor struct {
	l     *internal.Logger
	stats *internal.Stats

	in connector.Connector[*adapter.CANMessageBatch]
}

func NewProcessor() *Processor {
	l := internal.NewLogger("processor", "acmelib")

	return &Processor{
		l:     l,
		stats: internal.NewStats(l),
	}
}

func (p *Processor) Init(ctx context.Context) error {
	return nil
}

func (p *Processor) Run(ctx context.Context) {
	p.l.Info("starting run")
	defer p.l.Info("quitting run")

	go p.stats.RunStats(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		msg, err := p.in.Read()
		if err != nil {
			p.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		p.stats.IncrementItemCount()

		p.process(ctx, msg)
	}
}

func (p *Processor) Stop() {}

func (p *Processor) SetInput(connector connector.Connector[*adapter.CANMessageBatch]) {
	p.in = connector
}

func (p *Processor) process(_ context.Context, _ *adapter.CANMessageBatch) {
	// p.l.Info("processing message", "message", msg.String())
}
