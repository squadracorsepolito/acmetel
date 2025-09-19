package processor

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/stage"
	"go.opentelemetry.io/otel/attribute"
)

//////////////
//  CONFIG  //
//////////////

type CannelloniConfig struct {
	PoolConfig *pool.Config
}

func DefaultCannelloniConfig() *CannelloniConfig {
	return &CannelloniConfig{
		PoolConfig: pool.DefaultConfig(),
	}
}

///////////////
//  MESSAGE  //
///////////////

var _ message.ReOrderable = (*CannelloniMessage)(nil)
var _ CANMessageCarrier = (*CannelloniMessage)(nil)

const (
	// maximum number of CAN 2.0 messages (8 bytes payload) that can be sent in a single udp/ipv4/ethernet packet
	defaultCANMessageNum = 113
)

// CannelloniMessage represents a cannelloni CAN message.
type CannelloniMessage struct {
	message.Base

	seqNum uint8

	// Messages is the list of CAN messages contained in a cannelloni frame.
	Messages []CANRawMessage
	// MessageCount is the number of CAN messages.
	MessageCount int
}

func newCannelloniMessage() *CannelloniMessage {
	return &CannelloniMessage{
		MessageCount: 0,
		Messages:     make([]CANRawMessage, defaultCANMessageNum),
	}
}

// GetSequenceNumber returns the sequence number of the cannelloni frame.
func (cm *CannelloniMessage) GetSequenceNumber() uint64 {
	return uint64(cm.seqNum)
}

// GetRawMessages returns the list of CAN messages contained in the cannelloni frame.
func (cm *CannelloniMessage) GetRawMessages() []CANRawMessage {
	return cm.Messages[:cm.MessageCount]
}

///////////////
//  DECODER  //
///////////////

type cannelloniDecoder struct{}

func newCannelloniDecoder() *cannelloniDecoder {
	return &cannelloniDecoder{}
}

func (cd *cannelloniDecoder) decode(buf []byte) (*cannelloniFrame, error) {
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
		n, err := cd.decodeMessage(buf[pos:], &f.messages[i])
		if err != nil {
			return nil, err
		}

		pos += n
	}

	return &f, nil
}

func (cd *cannelloniDecoder) decodeMessage(buf []byte, msg *cannelloniFrameMessage) (int, error) {
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
		msg.canFDFlags = buf[5]
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

///////////////
//  ENCODER  //
///////////////

type cannelloniEncoder struct{}

func newCannelloniEncoder() *cannelloniEncoder {
	return &cannelloniEncoder{}
}

func (ce *cannelloniEncoder) encode(frame *cannelloniFrame) []byte {
	totMsgSize := int(frame.messageCount * 5)
	for _, msg := range frame.messages {
		if msg.canFDFlags != 0 {
			totMsgSize += int(msg.dataLen) + 1
			continue
		}

		totMsgSize += int(msg.dataLen)
	}

	buf := make([]byte, 5+totMsgSize)

	buf[0] = frame.version
	buf[1] = frame.opCode
	buf[2] = frame.sequenceNumber
	binary.BigEndian.PutUint16(buf[3:5], frame.messageCount)

	pos := 5
	for _, msg := range frame.messages {
		n := ce.encodeMessage(&msg, buf[pos:])
		pos += n
	}

	return buf
}

func (ce *cannelloniEncoder) encodeMessage(msg *cannelloniFrameMessage, buf []byte) int {
	n := 5

	binary.BigEndian.PutUint32(buf[0:4], msg.canID)

	buf[4] = msg.dataLen

	if msg.canFDFlags != 0 {
		buf[4] |= 0x80
		buf[5] = msg.canFDFlags
		n++
	}

	tmpDataLen := int(msg.dataLen)
	copy(buf[n:n+tmpDataLen], msg.data)
	n += tmpDataLen

	return n
}

//////////////
//  WORKER  //
//////////////

type cannelloniFrameMessage struct {
	canID      uint32
	dataLen    uint8
	canFDFlags uint8
	data       []byte
}

type cannelloniFrame struct {
	version        uint8
	opCode         uint8
	sequenceNumber uint8
	messageCount   uint16
	messages       []cannelloniFrameMessage
}

type cannelloniWorker[T msgSer] struct {
	tel *internal.Telemetry

	decoder *cannelloniDecoder
}

func (cw *cannelloniWorker[T]) SetTelemetry(tel *internal.Telemetry) {
	cw.tel = tel
}

func (cw *cannelloniWorker[T]) Init(_ context.Context, _ any) error {
	cw.decoder = newCannelloniDecoder()

	return nil
}

func (cw *cannelloniWorker[T]) Handle(ctx context.Context, msgIn T) (*CannelloniMessage, error) {
	// Extract the span context from the input message
	_, span := cw.tel.NewTrace(msgIn.LoadSpanContext(ctx), "handle cannelloni frame")
	defer span.End()

	// Decode the frame
	f, err := cw.decoder.decode(msgIn.GetBytes())
	if err != nil {
		return nil, err
	}

	// Create the cannelloni message with the decoded frame data
	cannelloniMsg := newCannelloniMessage()

	// Set the receive time, but ignore the timestamp
	// because it will be set by the rob
	cannelloniMsg.SetReceiveTime(msgIn.GetReceiveTime())

	cannelloniMsg.seqNum = f.sequenceNumber

	messageCount := len(f.messages)
	cannelloniMsg.MessageCount = messageCount
	if messageCount > defaultCANMessageNum {
		cannelloniMsg.Messages = make([]CANRawMessage, messageCount)
	}

	for idx, tmpMsg := range f.messages {
		cannelloniMsg.Messages[idx] = CANRawMessage{
			CANID:   tmpMsg.canID,
			DataLen: int(tmpMsg.dataLen),
			RawData: tmpMsg.data,
		}
	}

	// Save the span into the message
	span.SetAttributes(attribute.Int("message_count", messageCount))
	cannelloniMsg.SaveSpan(span)

	return cannelloniMsg, nil
}

func (cw *cannelloniWorker[T]) Close(_ context.Context) error {
	return nil
}

// func (cw *cannelloniWorker[T]) decodeFrame(buf []byte) (*cannelloniFrame, error) {
// 	if buf == nil {
// 		return nil, errors.New("nil buffer")
// 	}

// 	if len(buf) < 5 {
// 		return nil, errors.New("not enough data")
// 	}

// 	f := cannelloniFrame{
// 		version:        buf[0],
// 		opCode:         buf[1],
// 		sequenceNumber: buf[2],
// 		messageCount:   binary.BigEndian.Uint16(buf[3:5]),
// 	}

// 	f.messages = make([]cannelloniFrameMessage, f.messageCount)
// 	pos := 5
// 	for i := uint16(0); i < f.messageCount; i++ {
// 		n, err := cw.decodeFrameMessage(buf[pos:], &f.messages[i])
// 		if err != nil {
// 			return nil, err
// 		}

// 		pos += n
// 	}

// 	return &f, nil
// }

// func (cw *cannelloniWorker[T]) decodeFrameMessage(buf []byte, msg *cannelloniFrameMessage) (int, error) {
// 	if len(buf) < 5 {
// 		return 0, errors.New("not enough data")
// 	}

// 	n := 5

// 	msg.canID = binary.BigEndian.Uint32(buf[0:4])

// 	isCANFD := false
// 	tmpDataLen := buf[4]
// 	if tmpDataLen|0x80 == 0x80 {
// 		isCANFD = true
// 	}

// 	if isCANFD {
// 		if len(buf) < 6 {
// 			return 0, errors.New("not enough data")
// 		}

// 		msg.dataLen = tmpDataLen & 0x7f
// 		msg.canFDFlags = buf[5]
// 		n++
// 	} else {
// 		msg.dataLen = tmpDataLen
// 	}

// 	if len(buf) < n+int(tmpDataLen) {
// 		return 0, errors.New("not enough data for message content")
// 	}

// 	msg.data = make([]byte, tmpDataLen)

// 	copy(msg.data, buf[n:n+int(tmpDataLen)])
// 	n += int(msg.dataLen)

// 	return n, nil
// }

/////////////
//  STAGE  //
/////////////

type CannelloniStage[T msgSer] struct {
	*stage.Processor[T, *CannelloniMessage, cannelloniWorker[T], any, *cannelloniWorker[T]]

	cfg *CannelloniConfig
}

func NewCannelloniStage[T msgSer](inputConnector conn[T], outputConnector conn[*CannelloniMessage], cfg *CannelloniConfig) *CannelloniStage[T] {
	return &CannelloniStage[T]{
		Processor: stage.NewProcessor[T, *CannelloniMessage, cannelloniWorker[T], any](
			"cannelloni", inputConnector, outputConnector, cfg.PoolConfig,
		),

		cfg: cfg,
	}
}

func (cs *CannelloniStage[T]) Init(ctx context.Context) error {
	return cs.Processor.Init(ctx, nil)
}
