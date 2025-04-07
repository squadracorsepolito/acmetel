package internal

import (
	"context"
	"sync/atomic"
	"time"
)

type Stats struct {
	l *Logger

	itemCount atomic.Uint64
	byteCount atomic.Uint64
}

func NewStats(l *Logger) *Stats {
	return &Stats{
		l: l,
	}
}

func (s *Stats) RunStats(ctx context.Context) {
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

func (s *Stats) IncrementItemCount() {
	s.itemCount.Add(1)
}

func (s *Stats) IncrementByteCountBy(n int) {
	s.byteCount.Add(uint64(n))
}
