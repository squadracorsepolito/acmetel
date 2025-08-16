package pool

import (
	"runtime"
	"time"
)

type Config struct {
	AutoScale           bool
	InitialWorkers      int
	MinWorkers          int
	MaxWorkers          int
	QueueDepthPerWorker int
	ScaleDownFactor     float64
	ScaleDownBackoff    float64
	AutoScaleInterval   time.Duration
}

func DefaultConfig() *Config {
	return &Config{
		AutoScale:           true,
		InitialWorkers:      1,
		MinWorkers:          1,
		MaxWorkers:          runtime.NumCPU(),
		QueueDepthPerWorker: 128,
		ScaleDownFactor:     0.1,
		ScaleDownBackoff:    1.5,
		AutoScaleInterval:   3 * time.Second,
	}
}

func (cfg *Config) toScaler() *scalerCfg {
	return &scalerCfg{
		enabled:             cfg.AutoScale,
		maxWorkers:          cfg.MaxWorkers,
		minWorkers:          cfg.MinWorkers,
		queueDepthThreshold: float64(cfg.QueueDepthPerWorker),
		scaleDownFactor:     cfg.ScaleDownFactor,
		scaleDownBackoff:    cfg.ScaleDownBackoff,
		interval:            cfg.AutoScaleInterval,
	}
}

func (cfg *Config) ToPoolConfig() *Config {
	return cfg
}

type ConfigurablePool interface {
	ToPoolConfig() *Config
}
