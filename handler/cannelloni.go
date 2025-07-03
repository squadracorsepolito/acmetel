package handler

import (
	"context"
	"encoding/binary"
	"errors"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
	"go.opentelemetry.io/otel/attribute"
)

var cannelloniROBConfig = &internal.ROBConfig{
	WindowSize: 128,
	MaxSeqNum:  255,

	TimeAnomalyLowerBound: time.Millisecond * 500,
	TimeAnomalyUpperBound: time.Second * 5,

	FallbackInterval: time.Millisecond * 1,

	MaxSamples:              512,
	KeptSamples:             128,
	SampleEstimateTreshold:  256,
	SampleEstimateFrequency: 64,
}

type CannelloniConfig struct {
	*worker.PoolConfig

	ROBTimeout time.Duration
}

func NewDefaultCannelloniConfig() *CannelloniConfig {
	return &CannelloniConfig{
		PoolConfig: worker.DefaultPoolConfig(),

		ROBTimeout: 50 * time.Millisecond,
	}
}

type Cannelloni struct {
	// *robStage[*message.UDPPayload, *message.RawCANMessageBatch, *CannelloniConfig, cannelloniWorker, any, *cannelloniWorker]

	*stage[*message.UDPPayload, *message.RawCANMessageBatch, *CannelloniConfig, cannelloniWorker, any, *cannelloniWorker]
}

func NewCannelloni(cfg *CannelloniConfig) *Cannelloni {
	return &Cannelloni{
		// robStage: newROBStage[*message.UDPPayload, *message.RawCANMessageBatch, *CannelloniConfig, cannelloniWorker, any](
		// 	"cannelloni", cfg, cannelloniROBConfig, cfg.ROBTimeout),

		stage: newStage[*message.UDPPayload, *message.RawCANMessageBatch, *CannelloniConfig, cannelloniWorker, any]("cannelloni", cfg),
	}
}

func (h *Cannelloni) Init(ctx context.Context) error {
	return h.init(ctx, nil)
}

func (h *Cannelloni) Run(ctx context.Context) {
	h.run(ctx)
}

func (h *Cannelloni) Stop() {
	h.close()
}

type cannelloniFrameMessage struct {
	canID     uint32
	dataLen   uint8
	canDFlags uint8
	data      []byte
}

type cannelloniFrame struct {
	version        uint8
	opCode         uint8
	sequenceNumber uint8
	messageCount   uint16
	messages       []cannelloniFrameMessage
}

type cannelloniWorker struct {
	tel *internal.Telemetry
}

func (w *cannelloniWorker) Init(_ context.Context, _ any) error {
	return nil
}

func (w *cannelloniWorker) Handle(ctx context.Context, udpPayload *message.UDPPayload) (*message.RawCANMessageBatch, error) {
	ctx, span := w.tel.NewTrace(ctx, "process cannelloni frame")
	defer span.End()

	f, err := w.decodeFrame(udpPayload.Data)
	if err != nil {
		return nil, err
	}

	msgBatch := message.NewRawCANMessageBatch()
	msgBatch.SeqNum = f.sequenceNumber
	msgBatch.Timestamp = time.Now()

	messageCount := len(f.messages)
	msgBatch.MessageCount = messageCount
	if messageCount > message.DefaultCANMessageNum {
		msgBatch.Messages = make([]message.RawCANMessage, messageCount)
	}

	for idx, tmpMsg := range f.messages {
		msgBatch.Messages[idx] = message.RawCANMessage{
			CANID:   tmpMsg.canID,
			DataLen: int(tmpMsg.dataLen),
			RawData: tmpMsg.data,
		}
	}

	span.SetAttributes(attribute.Int("message_count", messageCount))

	return msgBatch, nil
}

func (w *cannelloniWorker) Stop(_ context.Context) error {
	return nil
}

func (w *cannelloniWorker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *cannelloniWorker) decodeFrame(buf []byte) (*cannelloniFrame, error) {
	if buf == nil {
		return nil, errors.New("nil buffer")
	}

	if len(buf) < 5 {
		return nil, errors.New("not enough data")
	}

	f := cannelloniFrame{
		version:        buf[0],
		opCode:         buf[1],
		sequenceNumber: buf[2],
		messageCount:   binary.BigEndian.Uint16(buf[3:5]),
	}

	f.messages = make([]cannelloniFrameMessage, f.messageCount)
	pos := 5
	for i := uint16(0); i < f.messageCount; i++ {
		n, err := w.decodeFrameMessage(buf[pos:], &f.messages[i])
		if err != nil {
			return nil, err
		}

		pos += n
	}

	return &f, nil
}

func (w *cannelloniWorker) decodeFrameMessage(buf []byte, msg *cannelloniFrameMessage) (int, error) {
	if len(buf) < 5 {
		return 0, errors.New("not enough data")
	}

	n := 5

	msg.canID = binary.BigEndian.Uint32(buf[0:4])

	isCANFD := false
	tmpDataLen := buf[4]
	if tmpDataLen|0x80 == 0x80 {
		isCANFD = true
	}

	if isCANFD {
		if len(buf) < 6 {
			return 0, errors.New("not enough data")
		}

		msg.dataLen = tmpDataLen & 0x7f
		msg.canDFlags = buf[5]
		n++
	} else {
		msg.dataLen = tmpDataLen
	}

	if len(buf) < n+int(tmpDataLen) {
		return 0, errors.New("not enough data for message content")
	}

	msg.data = make([]byte, tmpDataLen)

	copy(msg.data, buf[n:n+int(tmpDataLen)])
	n += int(msg.dataLen)

	return n, nil
}
