package udp

import (
	"github.com/squadracorsepolito/acmetel/internal/pool"
)

type Config struct {
	PoolConfig *pool.Config

	IPAddr string
	Port   uint16
}

func NewDefaultConfig() *Config {
	return &Config{
		PoolConfig: pool.DefaultConfig(),

		IPAddr: "127.0.0.1",
		Port:   20_000,
	}
}
