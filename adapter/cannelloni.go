package adapter

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"
	"time"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/ingress"
	"github.com/squadracorsepolito/acmetel/internal"
)

const (
	// maximum number of CAN 2.0 messages (8 bytes payload) that can be sent in a single udp/ipv4/ethernet packet
	defaultCANMessageNum = 113
)

type CANMessage struct {
	CANID   uint32
	DataLen int
	RawData []byte
}

type CANMessageBatch struct {
	Timestamp    time.Time
	MessageCount int
	Messages     []CANMessage
}

type CANMessageBatchPool struct {
	pool *sync.Pool
}

var CANMessageBatchPoolInstance = newCANMessageBatchPool()

func newCANMessageBatchPool() *CANMessageBatchPool {
	return &CANMessageBatchPool{
		pool: &sync.Pool{
			New: func() any {
				return &CANMessageBatch{
					MessageCount: 0,
					Messages:     make([]CANMessage, defaultCANMessageNum),
				}
			},
		},
	}
}

func (p *CANMessageBatchPool) Get() *CANMessageBatch {
	return p.pool.Get().(*CANMessageBatch)
}

func (p *CANMessageBatchPool) Put(b *CANMessageBatch) {
	p.pool.Put(b)
}

type CannelloniConfig struct {
	*internal.WorkerPoolConfig
}

func NewDefaultCannelloniConfig() *CannelloniConfig {
	return &CannelloniConfig{
		WorkerPoolConfig: internal.NewDefaultWorkerPoolConfig(),
	}
}

type Cannelloni struct {
	l     *internal.Logger
	stats *internal.Stats

	in connector.Connector[*ingress.UDPData]

	writerWg *sync.WaitGroup
	out      connector.Connector[*CANMessageBatch]

	workerPool *internal.WorkerPool[*ingress.UDPData, *CANMessageBatch]
}

func NewCannelloni(cfg *CannelloniConfig) *Cannelloni {
	l := internal.NewLogger("adapter", "cannelloni")

	return &Cannelloni{
		l:     l,
		stats: internal.NewStats(l),

		writerWg: &sync.WaitGroup{},

		workerPool: internal.NewWorkerPool(l, newCannelloniWorkerGen(), cfg.WorkerPoolConfig),
	}
}

func (c *Cannelloni) Init(_ context.Context) error {
	return nil
}

func (c *Cannelloni) runWriter(ctx context.Context) {
	c.writerWg.Add(1)
	defer c.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case data := <-c.workerPool.OutputCh:
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

		data, err := c.in.Read()
		if err != nil {
			c.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		c.stats.IncrementItemCount()
		c.stats.IncrementByteCountBy(len(data.Frame))

		received++

		if !c.workerPool.AddTask(ctx, data) {
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

func (c *Cannelloni) SetInput(connector connector.Connector[*ingress.UDPData]) {
	c.in = connector
}

func (c *Cannelloni) SetOutput(connector connector.Connector[*CANMessageBatch]) {
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

type cannelloniWorker struct {
}

func newCannelloniWorkerGen() internal.WorkerGen[*ingress.UDPData, *CANMessageBatch] {
	return func() internal.Worker[*ingress.UDPData, *CANMessageBatch] {
		return &cannelloniWorker{}
	}
}

func (w *cannelloniWorker) DoWork(_ context.Context, udpData *ingress.UDPData) (*CANMessageBatch, error) {
	f, err := w.decodeFrame(udpData.Frame)
	if err != nil {
		return nil, err
	}

	// ingress.UDPDataPoolInstance.Put(udpData)

	msgBatch := CANMessageBatchPoolInstance.Get()
	msgBatch.Timestamp = time.Now()

	messageCount := len(f.messages)
	msgBatch.MessageCount = messageCount
	if messageCount > defaultCANMessageNum {
		msgBatch.Messages = make([]CANMessage, messageCount)
	}

	for idx, tmpMsg := range f.messages {
		msgBatch.Messages[idx] = CANMessage{
			CANID:   tmpMsg.canID,
			DataLen: int(tmpMsg.dataLen),
			RawData: tmpMsg.data,
		}
	}

	return msgBatch, nil
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
