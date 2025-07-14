package udp

import w "github.com/squadracorsepolito/acmetel/worker"

type Config struct {
	*w.PoolConfig

	IPAddr string
	Port   uint16
}

func NewDefaultConfig() *Config {
	return &Config{
		PoolConfig: w.DefaultPoolConfig(),

		IPAddr: "127.0.0.1",
		Port:   20_000,
	}
}
