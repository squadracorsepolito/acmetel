package acmetel

import (
	"context"
	"errors"
	"time"

	"github.com/squadracorsepolito/acmetel/core"
	"github.com/squadracorsepolito/acmetel/internal"
)

type Processor struct {
	l *logger

	in *internal.RingBuffer[*core.Message]

	processedMsgCount int
}

func NewProcessor() *Processor {
	return &Processor{
		l: newLogger(stageKindProcessor, "processor"),
	}
}

func (p *Processor) Init(ctx context.Context) error {
	if p.in == nil {
		return errors.New("input connector not set")
	}

	return nil
}

func (p *Processor) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	p.l.Info("starting run")
	defer p.l.Info("quitting run")

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			if p.processedMsgCount == 0 {
				continue
			}

			p.l.Info("stats", "msg_per_sec", p.processedMsgCount)
			p.processedMsgCount = 0

		default:
		}

		msg, err := p.in.Read(ctx)
		if err != nil {
			p.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		p.process(ctx, msg)
	}
}

func (p *Processor) Stop() {}

func (p *Processor) SetInput(connector *internal.RingBuffer[*core.Message]) {
	p.in = connector
}

func (p *Processor) process(_ context.Context, _ *core.Message) {
	// p.l.Info("processing message", "message", msg.String())
	p.processedMsgCount++
}

// type processorWorker struct {
// 	l *slog.Logger

// 	id    int
// 	wg    *sync.WaitGroup
// 	msgCh <-chan *core.Message

// 	processedMsgCount int
// }

// func newProcessorWorker(id int, wg *sync.WaitGroup, msgCh <-chan *core.Message) *processorWorker {
// 	return &processorWorker{
// 		l: slog.Default(),

// 		id:    id,
// 		wg:    wg,
// 		msgCh: msgCh,
// 	}
// }

// func (w *processorWorker) run(ctx context.Context) {
// 	defer w.wg.Done()

// 	ticker := time.NewTicker(1 * time.Second)
// 	defer ticker.Stop()

// 	for {
// 		select {
// 		case <-ctx.Done():
// 			w.l.Info("processor worker stopped", "id", w.id, "reason", ctx.Err())
// 			return

// 		case msg := <-w.msgCh:
// 			w.process(msg)

// 		case <-ticker.C:
// 			if w.processedMsgCount == 0 {
// 				continue
// 			}

// 			w.l.Info("processor worker stats", "id", w.id, "processedMsgCountPerSec", w.processedMsgCount/10)
// 			w.processedMsgCount = 0
// 		}
// 	}
// }

// func (w *processorWorker) process(msg *core.Message) {
// 	w.processedMsgCount++
// 	// w.l.Info("processing message", "id", w.id, "message", msg.String())
// }
