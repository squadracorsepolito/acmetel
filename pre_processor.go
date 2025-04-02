package acmetel

import (
	"context"
	"errors"
	"time"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/cannelloni"
	"github.com/squadracorsepolito/acmetel/core"
	"github.com/squadracorsepolito/acmetel/internal"
)

type CannelloniPreProcessor struct {
	*stats

	l *logger

	in  *internal.RingBuffer[[]byte]
	out *internal.RingBuffer[*core.Message]
}

func NewCannelloniPreProcessor() *CannelloniPreProcessor {
	l := newLogger(stageKindPreProcessor, "cannelloni-pre-processor")

	return &CannelloniPreProcessor{
		stats: newStats(l),

		l: l,
	}
}

func (p *CannelloniPreProcessor) Init(ctx context.Context) error {
	if p.out == nil {
		return errors.New("output connector not set")
	}

	p.out.Init(ctx)

	if p.in == nil {
		return errors.New("input connector not set")
	}

	return nil
}

func (p *CannelloniPreProcessor) Run(ctx context.Context) {
	p.l.Info("starting run")
	defer p.l.Info("quitting run")

	go p.stats.runStats(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		buf, err := p.in.Read(ctx)
		if err != nil {
			p.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		p.incrementItemCount()
		p.incrementByteCountBy(len(buf))

		f, err := cannelloni.DecodeFrame(buf)
		if err != nil {
			p.l.Warn("failed to decode frame", "reason", err)
			continue
		}

		timestamp := time.Now()
		for _, msg := range f.Messages {
			tmpMsg := core.NewMessage(timestamp, acmelib.CANID(msg.CANID), int(msg.DataLen), msg.Data)

			if err := p.out.Write(ctx, tmpMsg); err != nil {
				p.l.Warn("failed to write into output connector", "reason", err)
			}
		}
	}
}

func (p *CannelloniPreProcessor) Stop() {}

func (p *CannelloniPreProcessor) Duplicate() ScalableStage {
	dup := NewCannelloniPreProcessor()

	dup.in = p.in
	dup.out = p.out

	return dup
}

func (p *CannelloniPreProcessor) SetID(id int) {
	p.l.SetStageID(id)
}

func (p *CannelloniPreProcessor) SetInput(connector *internal.RingBuffer[[]byte]) {
	p.in = connector
}

func (p *CannelloniPreProcessor) SetOutput(connector *internal.RingBuffer[*core.Message]) {
	p.out = connector
}
