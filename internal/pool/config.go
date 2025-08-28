// Package pool contains the worker pool implementations for the different kind of stages.
package pool

import (
	"runtime"
	"time"
)

// Config is the configuration for the worker pool.
type Config struct {
	// AutoScaleEnabled states whether the worker pool should scale automatically.
	//
	// Default: true
	AutoScaleEnabled bool `yaml:"auto_scale_enabled" json:"auto_scale_enabled"`

	// InitialWorkers is the initial number of workers.
	//
	// Default: 1
	InitialWorkers int `yaml:"initial_workers" json:"initial_workers"`

	// MinWorkers is the minimum number of workers.
	//
	// Default: 1
	MinWorkers int `yaml:"min_workers" json:"min_workers"`
	// MaxWorkers is the maximum number of workers.
	//
	// Default: number of CPUs
	MaxWorkers int `yaml:"max_workers" json:"max_workers"`

	// InputQueueSize is the size of the queue that holds messages to be processed
	// by the workers. It is basically the size of the buffer used to fan out the
	// messages to the workers.
	//
	// Default: 512
	InputQueueSize int `yaml:"input_queue_size" json:"input_queue_size"`

	// OutputQueueSize is the size of the queue that holds messages which have been
	// processed by the workers. It is basically the size of the buffer used to fan in
	// the messages from the workers. It is NOT used by the egress stage.
	//
	// Default: 512
	OutputQueueSize int `yaml:"output_queue_size" json:"output_queue_size"`

	// QueueDepthPerWorker is the target length of the task queue per worker.
	//
	// Default: 64
	QueueDepthPerWorker int `yaml:"queue_depth_per_worker" json:"queue_depth_per_worker"`

	// ScaleDownFactor is the factor by which to scale down the number of workers.
	//
	// Default: 0.1
	ScaleDownFactor float64 `yaml:"scale_down_factor" json:"scale_down_factor"`
	// ScaleDownBackoff is the factor by which to increase the time to scale down.
	//
	// Default: 1.5
	ScaleDownBackoff float64 `yaml:"scale_down_backoff" json:"scale_down_backoff"`

	// AutoScaleInterval is the interval at which the auto scaler is triggered.
	//
	// Default: 3 seconds
	AutoScaleInterval time.Duration `yaml:"auto_scale_interval" json:"auto_scale_interval"`
}

// DefaultConfig returns the default configuration for the worker pool.
func DefaultConfig() *Config {
	return &Config{
		AutoScaleEnabled:    true,
		InitialWorkers:      max(1, runtime.NumCPU()/2),
		MinWorkers:          1,
		MaxWorkers:          runtime.NumCPU(),
		InputQueueSize:      512,
		OutputQueueSize:     512,
		QueueDepthPerWorker: 64,
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
