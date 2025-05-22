package adapter

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
	"go.opentelemetry.io/otel/attribute"
)

type CannelloniConfig struct {
	*worker.PoolConfig
}

func NewDefaultCannelloniConfig() *CannelloniConfig {
	return &CannelloniConfig{
		PoolConfig: worker.DefaultPoolConfig(),
	}
}

type Cannelloni struct {
	l   *internal.Logger
	tel *internal.Telemetry

	stats *internal.Stats

	in connector.Connector[*message.UDPPayload]

	writerWg *sync.WaitGroup
	out      connector.Connector[*message.RawCANMessageBatch]

	workerPool *cannelloniWorkerPool
}

func NewCannelloni(cfg *CannelloniConfig) *Cannelloni {
	tel := internal.NewTelemetry("adapter", "cannelloni")

	l := tel.Logger()

	return &Cannelloni{
		l:   l,
		tel: tel,

		stats: internal.NewStats(l),

		writerWg: &sync.WaitGroup{},

		workerPool: worker.NewPool[cannelloniWorker, any, *message.UDPPayload, *message.RawCANMessageBatch](tel, cfg.PoolConfig),
	}
}

func (c *Cannelloni) Init(ctx context.Context) error {
	c.workerPool.Init(ctx, nil)

	return nil
}

func (c *Cannelloni) runWriter(ctx context.Context) {
	c.writerWg.Add(1)
	defer c.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case data := <-c.workerPool.GetOutputCh():
			if err := c.out.Write(data); err != nil {
				c.l.Warn("failed to write into output connector", "reason", err)
			}
		}
	}
}

func (c *Cannelloni) Run(ctx context.Context) {
	c.l.Info("running")

	received := 0
	skipped := 0
	defer func() {
		c.l.Info("received frames", "count", received)
		c.l.Info("skipped frames", "count", skipped)
	}()

	go c.stats.RunStats(ctx)

	go c.workerPool.Run(ctx)

	go c.runWriter(ctx)

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		udpPayload, err := c.in.Read()
		if err != nil {
			c.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		c.stats.IncrementItemCount()
		c.stats.IncrementByteCountBy(udpPayload.DataLen)

		received++

		if !c.workerPool.AddTask(ctx, udpPayload) {
			skipped++
		}
	}
}

func (c *Cannelloni) Stop() {
	defer c.l.Info("stopped")

	c.out.Close()
	c.workerPool.Stop()
	c.writerWg.Wait()
}

func (c *Cannelloni) SetInput(connector connector.Connector[*message.UDPPayload]) {
	c.in = connector
}

func (c *Cannelloni) SetOutput(connector connector.Connector[*message.RawCANMessageBatch]) {
	c.out = connector
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

type cannelloniWorkerPool = worker.Pool[cannelloniWorker, any, *message.UDPPayload, *message.RawCANMessageBatch, *cannelloniWorker]

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
