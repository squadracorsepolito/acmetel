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

type CANMessageBatch struct {
	Timestamp time.Time
	Messages  []CANMessage
}

type CANMessage struct {
	CANID   uint32
	DataLen int
	RawData []byte
}

type CannelloniConfig struct {
	WorkerNum int
}

func DefaultCannelloniConfig() *CannelloniConfig {
	return &CannelloniConfig{
		WorkerNum: 1,
	}
}

type Cannelloni struct {
	l     *internal.Logger
	stats *internal.Stats

	in connector.Connector[*ingress.UDPData]

	pool *sync.Pool
	out  connector.Connector[*CANMessageBatch]

	workerNum int
	workerWg  *sync.WaitGroup
	workerCh  chan *ingress.UDPData
}

func NewCannelloni(cfg *CannelloniConfig) *Cannelloni {
	l := internal.NewLogger("adapter", "cannelloni")

	return &Cannelloni{
		l:     l,
		stats: internal.NewStats(l),

		pool: &sync.Pool{
			New: func() any {
				return &CANMessageBatch{
					Messages: make([]CANMessage, defaultCANMessageNum),
				}
			},
		},

		workerNum: cfg.WorkerNum,
		workerWg:  &sync.WaitGroup{},
		workerCh:  make(chan *ingress.UDPData, cfg.WorkerNum*16),
	}
}

func (p *Cannelloni) Init(ctx context.Context) error {
	return nil
}

func (p *Cannelloni) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-p.workerCh:
			f, err := p.decodeFrame(data.Frame)
			if err != nil {
				p.l.Warn("failed to decode frame", "reason", err)
				continue
			}

			msgBatch := p.pool.Get().(*CANMessageBatch)
			msgBatch.Timestamp = time.Now()

			if len(f.messages) > defaultCANMessageNum {
				msgBatch.Messages = make([]CANMessage, len(f.messages))
			}

			for idx, tmpMsg := range f.messages {
				msgBatch.Messages[idx] = CANMessage{
					CANID:   tmpMsg.canID,
					DataLen: int(tmpMsg.dataLen),
					RawData: tmpMsg.data,
				}
			}

			if err := p.out.Write(msgBatch); err != nil {
				p.l.Warn("failed to write into output connector", "reason", err)
			}

			p.pool.Put(msgBatch)
		}
	}
}

func (p *Cannelloni) Run(ctx context.Context) {
	p.l.Info("starting run")
	defer p.l.Info("quitting run")

	go p.stats.RunStats(ctx)

	p.workerWg.Add(p.workerNum)

	for range p.workerNum {
		go func() {
			p.runWorker(ctx)
			p.workerWg.Done()
		}()
	}

	received := 0
	skipped := 0
	defer func() {
		p.l.Info("received frames", "count", received)
		p.l.Info("skipped frames", "count", skipped)
	}()

	for {
		select {
		case <-ctx.Done():
			return

		default:
		}

		data, err := p.in.Read()
		if err != nil {
			p.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		p.stats.IncrementItemCount()
		p.stats.IncrementByteCountBy(len(data.Frame))

		received++

		select {
		case p.workerCh <- data:
		default:
			skipped++
		}
	}
}

func (p *Cannelloni) Stop() {
	p.out.Close()

	p.workerWg.Wait()
	close(p.workerCh)
}

func (p *Cannelloni) SetInput(connector connector.Connector[*ingress.UDPData]) {
	p.in = connector
}

func (p *Cannelloni) SetOutput(connector connector.Connector[*CANMessageBatch]) {
	p.out = connector
}

func (p *Cannelloni) decodeFrame(buf []byte) (*cannelloniFrame, error) {
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
		n, err := p.decodeFrameMessage(buf[pos:], &f.messages[i])
		if err != nil {
			return nil, err
		}

		pos += n
	}

	return &f, nil
}

func (p *Cannelloni) decodeFrameMessage(buf []byte, msg *cannelloniFrameMessage) (int, error) {
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
