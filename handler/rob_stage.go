package handler

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
)

type robStage[MIn message.Message, MOut message.ReOrderableMessage, Cfg worker.ConfigurablePool, W, WArgs any, WPtr worker.HandlerWorkerPtr[W, WArgs, MIn, MOut]] struct {
	*stage[MIn, MOut, Cfg, W, WArgs, WPtr]

	rob        *internal.ROB[MOut]
	robTimeout time.Duration

	droppedMessages atomic.Int64
}

func newROBStage[MIn message.Message, MOut message.ReOrderableMessage, Cfg worker.ConfigurablePool, W, WArgs any, WPtr worker.HandlerWorkerPtr[W, WArgs, MIn, MOut]](
	name string, cfg Cfg, robCfg *internal.ROBConfig, robTimeout time.Duration) *robStage[MIn, MOut, Cfg, W, WArgs, WPtr] {

	stage := newStage[MIn, MOut, Cfg, W, WArgs, WPtr](name, cfg)

	return &robStage[MIn, MOut, Cfg, W, WArgs, WPtr]{
		stage: stage,

		rob:        internal.NewROB[MOut](stage.tel, robCfg),
		robTimeout: robTimeout,
	}
}

func (s *robStage[MIn, MOut, Cfg, W, WArgs, WPtr]) init(ctx context.Context, workerArgs WArgs) error {
	s.tel.LogInfo("initializing")
	defer s.tel.LogInfo("initialized")

	s.workerPool.Init(ctx, workerArgs)

	s.initMetrics()

	return nil
}

func (s *robStage[MIn, MOut, Cfg, W, WArgs, WPtr]) initMetrics() {
	s.stage.initMetrics()

	s.tel.NewCounter("dropped_messages", func() int64 { return s.droppedMessages.Load() })
}

func (s *robStage[MIn, MOut, Cfg, W, WArgs, WPtr]) run(ctx context.Context) {
	s.tel.LogInfo("running")
	defer s.tel.LogInfo("stopped")

	go s.workerPool.Run(ctx)

	go s.runROB(ctx)

	go s.runWriter(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		msg, err := s.inputConnector.Read()
		if err != nil {
			if errors.Is(err, connector.ErrClosed) {
				s.tel.LogInfo("input connector is closed, stopping")
				return
			}

			s.tel.LogError("failed to read from input connector", err)
			continue
		}

		if !s.workerPool.AddTask(ctx, msg) {
			s.skippedMessages.Add(1)
		}
	}
}

func (s *robStage[MIn, MOut, Cfg, W, WArgs, WPtr]) runROB(ctx context.Context) {
	flushTimeout := time.NewTimer(s.robTimeout)
	defer flushTimeout.Stop()

	poolOutputCh := s.workerPool.GetOutputCh()
	for {
		select {
		case <-ctx.Done():
			s.rob.Flush()
			return

		case <-flushTimeout.C:
			s.rob.Flush()
			flushTimeout.Reset(s.robTimeout)

		case msgOut := <-poolOutputCh:
			flushTimeout.Reset(s.robTimeout)

			if err := s.rob.Enqueue(msgOut); err != nil {
				s.tel.LogError("message dropped", err, "sequence_number", msgOut.SequenceNumber())
				s.droppedMessages.Add(1)
			}
		}
	}
}

func (s *robStage[MIn, MOut, Cfg, W, WArgs, WPtr]) runWriter(ctx context.Context) {
	s.writerWg.Add(1)
	defer s.writerWg.Done()

	robOutputCh := s.rob.GetOutputCh()
	for {
		select {
		case <-ctx.Done():
			return
		case msgOut := <-robOutputCh:
			if err := s.outputConnector.Write(msgOut); err != nil {
				s.tel.LogError("failed to write into output connector", err)
			}
		}
	}
}
