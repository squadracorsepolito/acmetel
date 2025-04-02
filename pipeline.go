package acmetel

import (
	"context"
	"sync"
)

type stageKind string

const (
	stageKindIngress      stageKind = "ingress"
	stageKindPreProcessor stageKind = "pre-processor"
	stageKindProcessor    stageKind = "processor"
)

type Stage interface {
	Init(ctx context.Context) error
	Run(ctx context.Context)
	Stop()
}

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

func (p *Pipeline) Stop() {
	for _, stage := range p.stages {
		stage.Stop()
	}

	p.wg.Wait()
}
