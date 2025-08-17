// Package pool contains the worker pool implementations for the different kind of stages.
package pool

import (
	"runtime"
	"time"
)

// Config is the configuration for the worker pool.
type Config struct {
	// AutoScaleEnabled states whether the worker pool should scale automatically.
	AutoScaleEnabled bool `yaml:"auto_scale_enabled" json:"auto_scale_enabled"`

	// InitialWorkers is the initial number of workers.
	InitialWorkers int `yaml:"initial_workers" json:"initial_workers"`

	// MinWorkers is the minimum number of workers.
	MinWorkers int `yaml:"min_workers" json:"min_workers"`
	// MaxWorkers is the maximum number of workers.
	MaxWorkers int `yaml:"max_workers" json:"max_workers"`

	// QueueDepthPerWorker is the target length of the task queue per worker.
	QueueDepthPerWorker int `yaml:"queue_depth_per_worker" json:"queue_depth_per_worker"`

	// ScaleDownFactor is the factor by which to scale down the number of workers.
	ScaleDownFactor float64 `yaml:"scale_down_factor" json:"scale_down_factor"`
	// ScaleDownBackoff is the factor by which to increase the time to scale down.
	ScaleDownBackoff float64 `yaml:"scale_down_backoff" json:"scale_down_backoff"`

	// AutoScaleInterval is the interval at which the auto scaler is triggered.
	AutoScaleInterval time.Duration `yaml:"auto_scale_interval" json:"auto_scale_interval"`
}

// DefaultConfig returns the default configuration for the worker pool.
func DefaultConfig() *Config {
	return &Config{
		AutoScaleEnabled:    true,
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
		enabled:             cfg.AutoScaleEnabled,
		maxWorkers:          cfg.MaxWorkers,
		minWorkers:          cfg.MinWorkers,
		queueDepthThreshold: float64(cfg.QueueDepthPerWorker),
		scaleDownFactor:     cfg.ScaleDownFactor,
		scaleDownBackoff:    cfg.ScaleDownBackoff,
		interval:            cfg.AutoScaleInterval,
	}
}
