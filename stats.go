package acmetel

import (
	"context"
	"sync/atomic"
	"time"
)

type stats struct {
	l *logger

	itemCount atomic.Uint64
	byteCount atomic.Uint64
}

func newStats(l *logger) *stats {
	return &stats{
		l: l,
	}
}

func (s *stats) runStats(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			itemCount := s.itemCount.Load()
			byteCount := s.byteCount.Load()

			if itemCount == 0 && byteCount == 0 {
				continue
			}

			s.itemCount.Store(0)
			s.byteCount.Store(0)

			s.l.Info("stats", "items_per_sec", itemCount, "bytes_per_sec", byteCount)
		}
	}
}

func (s *stats) incrementItemCount() {
	s.itemCount.Add(1)
}

func (s *stats) incrementByteCountBy(n int) {
	s.byteCount.Add(uint64(n))
}
