package acmetel

import (
	"context"
	"sync"

	"github.com/squadracorsepolito/acmetel/connector"
)

type Stage interface {
	Init(ctx context.Context) error
	Run(ctx context.Context)
	Close()
}

type Connector[T any] = connector.Connector[T]

type Pipeline struct {
	stages []Stage

	wg        *sync.WaitGroup
	isRunning bool
}

func NewPipeline() *Pipeline {
	return &Pipeline{
		stages: []Stage{},

		wg:        &sync.WaitGroup{},
		isRunning: false,
	}
}

func (p *Pipeline) AddStage(stage Stage) {
	if p.isRunning {
		return
	}

	p.stages = append(p.stages, stage)
}

func (p *Pipeline) Init(ctx context.Context) error {
	for _, stage := range p.stages {
		if err := stage.Init(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (p *Pipeline) Run(ctx context.Context) {
	p.isRunning = true

	p.wg.Add(len(p.stages))

	for _, stage := range p.stages {
		go func() {
			stage.Run(ctx)
			p.wg.Done()
		}()
	}
}

func (p *Pipeline) Close() {
	for _, stage := range p.stages {
		stage.Close()
	}

	p.wg.Wait()
}
