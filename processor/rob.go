package processor

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/rob"
)

//////////////
//  CONFIG  //
//////////////

type ROBConfig struct {
	ROB *rob.Config

	ResetTimeout time.Duration
}

func DefaultROBConfig() *ROBConfig {
	return &ROBConfig{
		ROB: &rob.Config{
			OutputChannelSize:   256,
			MaxSeqNum:           255,
			PrimaryBufferSize:   128,
			AuxiliaryBufferSize: 128,
			FlushTreshold:       0.3,
			BaseAlpha:           0.2,
			JumpThreshold:       8,
		},

		ResetTimeout: 50 * time.Millisecond,
	}
}

/////////////
//  STAGE  //
/////////////

type ROBStage[T message.ReOrderable] struct {
	tel *internal.Telemetry

	cfg *ROBConfig

	inputConnector  connector.Connector[T]
	outputConnector connector.Connector[T]

	rob *rob.ROB[T]

	// Metrics
	orderedMsgs           atomic.Int64
	primayEnqueuedMsgs    atomic.Int64
	auxiliaryEnqueuedMsgs atomic.Int64

	outOfOrderSeqNum atomic.Int64
	duplicatedSeqNum atomic.Int64
	invalidSeqNum    atomic.Int64

	resets atomic.Int64
}

func NewROBStage[T message.ReOrderable](inConnector connector.Connector[T], outConnector connector.Connector[T], cfg *ROBConfig) *ROBStage[T] {
	tel := internal.NewTelemetry("processor", "rob")

	inConnector.SetReadTimeout(cfg.ResetTimeout)

	return &ROBStage[T]{
		tel: tel,

		cfg: cfg,

		inputConnector:  inConnector,
		outputConnector: outConnector,

		rob: rob.NewROB(outConnector, cfg.ROB),
	}
}

func (rs *ROBStage[T]) Init(ctx context.Context) error {
	rs.tel.LogInfo("initializing")
	defer rs.tel.LogInfo("initialized")

	rs.initMetrics()

	return nil
}

func (rs *ROBStage[T]) initMetrics() {
	rs.tel.NewCounter("ordered_messages", func() int64 { return rs.orderedMsgs.Load() })
	rs.tel.NewCounter("primary_enqueued_messages", func() int64 { return rs.primayEnqueuedMsgs.Load() })
	rs.tel.NewCounter("auxiliary_enqueued_messages", func() int64 { return rs.auxiliaryEnqueuedMsgs.Load() })

	rs.tel.NewCounter("out_of_order_sequence_number", func() int64 { return rs.outOfOrderSeqNum.Load() })
	rs.tel.NewCounter("duplicated_sequence_number", func() int64 { return rs.duplicatedSeqNum.Load() })
	rs.tel.NewCounter("invalid_sequence_number", func() int64 { return rs.invalidSeqNum.Load() })

	rs.tel.NewCounter("resets", func() int64 { return rs.resets.Load() })
}

func (rs *ROBStage[T]) Run(ctx context.Context) {
	rs.tel.LogInfo("running")
	defer rs.tel.LogInfo("stopped")

	for {
		select {
		case <-ctx.Done():
			// Context is done, flush the ROB and return
			rs.rob.FlushAndReset()
			return

		default:
			lastRecvTime := time.Now()

			msgIn, err := rs.inputConnector.Read()
			if err != nil {
				if errors.Is(err, connector.ErrClosed) {
					return
				} else if errors.Is(err, connector.ErrReadTimeout) {
					// Timeout, reset and flush the ROB
					rs.rob.FlushAndReset()
					rs.resets.Add(1)
				} else {
					rs.tel.LogError("failed to read from input connector", err)
				}

				continue
			}

			if time.Since(lastRecvTime) >= rs.cfg.ResetTimeout {
				rs.rob.FlushAndReset()
				rs.resets.Add(1)
			}

			// Try to enqueue the message
			rs.enqueue(msgIn)
		}
	}
}

func (rs *ROBStage[T]) enqueue(msgIn T) {
	status, err := rs.rob.Enqueue(msgIn)
	if err != nil {
		if errors.Is(err, rob.ErrSeqNumOutOfWindow) {
			rs.outOfOrderSeqNum.Add(1)
		} else if errors.Is(err, rob.ErrSeqNumDuplicated) {
			rs.duplicatedSeqNum.Add(1)
		} else if errors.Is(err, rob.ErrSeqNumTooBig) {
			rs.invalidSeqNum.Add(1)
		}
	}

	switch status {
	case rob.EnqueueStatusInOrder:
		rs.orderedMsgs.Add(1)
	case rob.EnqueueStatusPrimary:
		rs.primayEnqueuedMsgs.Add(1)
	case rob.EnqueueStatusAuxiliary:
		rs.auxiliaryEnqueuedMsgs.Add(1)
	case rob.EnqueueStatusErr:
		return
	}
}

func (rs *ROBStage[T]) Close() {
	rs.tel.LogInfo("closing")
	defer rs.tel.LogInfo("closed")

	rs.outputConnector.Close()
}
