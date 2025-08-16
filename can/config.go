package can

import (
	"github.com/squadracorsepolito/acmelib"
	"github.com/squadracorsepolito/acmetel/internal/pool"
)

type Config struct {
	PoolConfig *pool.Config

	Messages []*acmelib.Message
}

func NewDefaultConfig() *Config {
	return &Config{
		PoolConfig: pool.DefaultConfig(),
	}
}
