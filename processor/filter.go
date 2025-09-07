package processor

import (
	"context"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

//////////////
//  CONFIG  //
//////////////

// FilterConfig structs contains the configuration for the [FilterStage].
type FilterConfig struct {
	PoolConfig *pool.Config `yaml:"pool_config" json:"pool_config"`
}

// DefaultFilterConfig returns the default configuration for the [FilterStage].
func DefaultFilterConfig() *FilterConfig {
	return &FilterConfig{
		PoolConfig: pool.DefaultConfig(),
	}
}

//////////////
//  WORKER  //
//////////////

type filterWorkerArgs[T msg] struct {
	filterFn func(T) bool
}

func newFilterWorkerArgs[T msg](filterFn func(T) bool) *filterWorkerArgs[T] {
	return &filterWorkerArgs[T]{
		filterFn: filterFn,
	}
}

type filterWorker[T msg] struct {
	pool.BaseWorker

	filterFn func(T) bool

	// Metrics
	filteredMessages atomic.Int64
}

func (fw *filterWorker[T]) Init(_ context.Context, args *filterWorkerArgs[T]) error {
	fw.filterFn = args.filterFn

	fw.initMetrics()

	return nil
}

func (fw *filterWorker[T]) initMetrics() {
	fw.Tel.NewCounter("filtered_messages", func() int64 { return fw.filteredMessages.Load() })
}

func (fw *filterWorker[T]) Handle(ctx context.Context, msgIn T) (T, error) {
	// Extract the span context from the input message
	ctx, span := fw.Tel.NewTrace(msgIn.LoadSpanContext(ctx), "filter message")
	defer span.End()

	if !fw.filterFn(msgIn) {
		msgIn.Drop()

		fw.filteredMessages.Add(1)
	}

	return msgIn, nil
}

func (fw *filterWorker[T]) Close(_ context.Context) error {
	return nil
}

/////////////
//  STAGE  //
/////////////

// FilterStage is a processor stage that filters messages based on a user-defined function.
type FilterStage[T msg] struct {
	*stage.Processor[T, T, filterWorker[T], *filterWorkerArgs[T], *filterWorker[T]]

	cfg *FilterConfig

	filterFn func(T) bool
}

// NewFilterStage returns a new filter processor stage.
func NewFilterStage[T msg](filterFn func(T) bool, inputConnector, outputConnector conn[T], cfg *FilterConfig) *FilterStage[T] {
	return &FilterStage[T]{
		Processor: stage.NewProcessor[T, T, filterWorker[T], *filterWorkerArgs[T]](
			"filter", inputConnector, outputConnector, cfg.PoolConfig,
		),

		cfg: cfg,

		filterFn: filterFn,
	}
}

// Init initializes the stage.
func (fs *FilterStage[T]) Init(ctx context.Context) error {
	return fs.Processor.Init(ctx, newFilterWorkerArgs(fs.filterFn))
}
