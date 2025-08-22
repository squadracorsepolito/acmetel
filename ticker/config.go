package ticker

import (
	"time"
)

type Config struct {
	WriterQueueSize int

	Interval time.Duration
}

func NewDefaultConfig() *Config {
	return &Config{
		WriterQueueSize: 256,

		Interval: 100 * time.Millisecond,
	}
}
