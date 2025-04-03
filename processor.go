package acmetel

import (
	"context"

	"github.com/squadracorsepolito/acmetel/core"
)

type Processor struct {
	*stats

	l *logger

	in Connector[core.Message]
}

func NewProcessor() *Processor {
	l := newLogger(stageKindProcessor, "processor")

	return &Processor{
		stats: newStats(l),

		l: l,
	}
}

func (p *Processor) Init(ctx context.Context) error {
	return nil
}

func (p *Processor) Run(ctx context.Context) {
	p.l.Info("starting run")
	defer p.l.Info("quitting run")

	go p.runStats(ctx)

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

		p.incrementItemCount()

		p.process(ctx, msg)
	}
}

func (p *Processor) Stop() {}

func (p *Processor) SetInput(connector Connector[core.Message]) {
	p.in = connector
}

func (p *Processor) process(_ context.Context, _ core.Message) {
	// p.l.Info("processing message", "message", msg.String())
}

func (p *Processor) Duplicate() ScalableStage {
	dup := NewProcessor()

	dup.in = p.in

	return dup
}

func (p *Processor) SetID(id int) {
	p.l.SetID(id)
}
