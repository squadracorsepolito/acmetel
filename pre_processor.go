package acmetel

import (
	"context"
	"time"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/cannelloni"
	"github.com/squadracorsepolito/acmetel/core"
)

type CannelloniPreProcessor struct {
	*stats

	l *logger

	in  Connector[[2048]byte]
	out Connector[core.Message]
}

func NewCannelloniPreProcessor() *CannelloniPreProcessor {
	l := newLogger(stageKindPreProcessor, "cannelloni-pre-processor")

	return &CannelloniPreProcessor{
		stats: newStats(l),

		l: l,
	}
}

func (p *CannelloniPreProcessor) Init(ctx context.Context) error {
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

		buf, err := p.in.Read()
		if err != nil {
			p.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		p.incrementItemCount()
		p.incrementByteCountBy(len(buf))

		f, err := cannelloni.DecodeFrame(buf[:])
		if err != nil {
			p.l.Warn("failed to decode frame", "reason", err)
			continue
		}

		timestamp := time.Now()
		for _, msg := range f.Messages {
			tmpMsg := core.NewMessage(timestamp, acmelib.CANID(msg.CANID), int(msg.DataLen), msg.Data)

			if err := p.out.Write(*tmpMsg); err != nil {
				p.l.Warn("failed to write into output connector", "reason", err)
			}
		}
	}
}

func (p *CannelloniPreProcessor) Stop() {
	p.out.Close()
}

func (p *CannelloniPreProcessor) Duplicate() ScalableStage {
	dup := NewCannelloniPreProcessor()

	dup.in = p.in
	dup.out = p.out

	return dup
}

func (p *CannelloniPreProcessor) SetID(id int) {
	p.l.SetID(id)
}

func (p *CannelloniPreProcessor) SetInput(connector Connector[[2048]byte]) {
	p.in = connector
}

func (p *CannelloniPreProcessor) SetOutput(connector Connector[core.Message]) {
	p.out = connector
}
