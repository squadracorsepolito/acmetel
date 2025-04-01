package acmetel

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/cannelloni"
	"github.com/squadracorsepolito/acmetel/core"
	"github.com/squadracorsepolito/acmetel/internal"
)

type CannelloniPreProcessor struct {
	l *logger

	in  *internal.RingBuffer[[]byte]
	out *internal.RingBuffer[*core.Message]

	frameCount atomic.Uint64
}

func NewCannelloniPreProcessor() *CannelloniPreProcessor {
	return &CannelloniPreProcessor{
		l: newLogger(stageKindPreProcessor, "cannelloni-pre-processor"),
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

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			framePerSec := p.frameCount.Load()
			if framePerSec != 0 {
				p.l.Info("stats", "frame_per_sec", framePerSec)
				p.frameCount.Store(0)
			}

		default:
		}

		buf, err := p.in.Read(ctx)
		if err != nil {
			p.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

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

		p.frameCount.Add(1)
	}
}

func (p *CannelloniPreProcessor) Stop() {}

func (p *CannelloniPreProcessor) SetInput(connector *internal.RingBuffer[[]byte]) {
	p.in = connector
}

func (p *CannelloniPreProcessor) SetOutput(connector *internal.RingBuffer[*core.Message]) {
	p.out = connector
}
