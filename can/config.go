package can

import (
	"github.com/squadracorsepolito/acmelib"
	w "github.com/squadracorsepolito/acmetel/worker"
)

type Config struct {
	*w.PoolConfig

	Messages []*acmelib.Message
}

func NewDefaultConfig() *Config {
	return &Config{
		PoolConfig: w.DefaultPoolConfig(),
	}
}
