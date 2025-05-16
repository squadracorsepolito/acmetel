package worker

import "context"

type Worker[InitArgs, In, Out any] interface {
	Init(ctx context.Context, args InitArgs) error
	DoWork(ctx context.Context, task In) (Out, error)
	Stop(ctx context.Context) error
}

type WorkerPtr[W, InitArgs, In, Out any] interface {
	*W
	Worker[InitArgs, In, Out]
}

type EgressWorker[InitArgs, In any] interface {
	Init(ctx context.Context, args InitArgs) error
	DoWork(ctx context.Context, task In) error
	Stop(ctx context.Context) error
}

type EgressWorkerPtr[W, InitArgs, In any] interface {
	*W
	EgressWorker[InitArgs, In]
}
