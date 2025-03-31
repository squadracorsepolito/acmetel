package cannelloni

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/core"
	"github.com/squadracorsepolito/acmetel/internal"
)

type Preprocessor struct {
	l *slog.Logger

	rb *internal.RingBuffer[[]byte]
}

func NewPreprocessor(rb *internal.RingBuffer[[]byte]) *Preprocessor {
	return &Preprocessor{
		l: slog.Default(),

		rb: rb,
	}
}

func (p *Preprocessor) Run(ctx context.Context, resCh chan<- *core.Message) {
	for {
		select {
		case <-ctx.Done():
			p.l.Info("cannelloni preprocessor stopped", "reason", ctx.Err())
			return
		default:
		}

		buf, err := p.rb.Get(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				p.l.Error("cannelloni preprocessor failed to get from ring buffer", "reason", err)
			}

			continue
		}

		f, err := DecodeFrame(buf)
		if err != nil {
			p.l.Error("cannelloni preprocessor failed to decode frame", "reason", err)
			continue
		}

		timestamp := time.Now()
		for _, msg := range f.Messages {
			resCh <- core.NewMessage(timestamp, acmelib.CANID(msg.CANID), int(msg.DataLen), msg.Data)
		}
	}
}
