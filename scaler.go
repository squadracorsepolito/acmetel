package acmetel

import (
	"context"
	"sync"
)

type ScalableStage interface {
	Stage

	Duplicate() ScalableStage
	SetID(id int)
}

type Scaler struct {
	workers     []ScalableStage
	workerCount int

	wg *sync.WaitGroup
}

func NewScaler(worker ScalableStage, workerCount int) *Scaler {
	workers := make([]ScalableStage, workerCount)
	workers[0] = worker

	for i := 1; i < workerCount; i++ {
		dup := worker.Duplicate()
		dup.SetID(i)
		workers[i] = dup
	}

	return &Scaler{
		workers:     workers,
		workerCount: workerCount,

		wg: &sync.WaitGroup{},
	}
}

func (s *Scaler) Init(ctx context.Context) error {
	for _, worker := range s.workers {
		if err := worker.Init(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (s *Scaler) Run(ctx context.Context) {
	s.wg.Add(s.workerCount)

	for _, worker := range s.workers {
		go func() {
			worker.Run(ctx)
			s.wg.Done()
		}()
	}
}

func (s *Scaler) Stop() {
	for _, worker := range s.workers {
		worker.Stop()
	}

	s.wg.Wait()
}
