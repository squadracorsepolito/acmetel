package cannelloni

import (
	"time"

	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/rob"
)

type Config struct {
	PoolConfig *pool.Config

	ROBConfig  *rob.Config
	ROBTimeout time.Duration
}

func NewDefaultConfig() *Config {
	return &Config{
		PoolConfig: pool.DefaultConfig(),

		ROBConfig: &rob.Config{
			OutputChannelSize:   256,
			MaxSeqNum:           255,
			PrimaryBufferSize:   128,
			AuxiliaryBufferSize: 128,
			FlushTreshold:       0.3,
			BaseAlpha:           0.2,
			JumpThreshold:       8,
		},

		ROBTimeout: 50 * time.Millisecond,
	}
}
