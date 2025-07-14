package questdb

import w "github.com/squadracorsepolito/acmetel/worker"

type Config struct {
	*w.PoolConfig

	Address string
}

func NewDefaultConfig() *Config {
	return &Config{
		PoolConfig: w.DefaultPoolConfig(),

		Address: "localhost:9000",
	}
}
