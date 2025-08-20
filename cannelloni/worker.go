package cannelloni

import (
	"context"
	"encoding/binary"
	"errors"

	"github.com/squadracorsepolito/acmetel/can"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"go.opentelemetry.io/otel/attribute"
)

type frameMessage struct {
	canID      uint32
	dataLen    uint8
	canFDFlags uint8
	data       []byte
}

type frame struct {
	version        uint8
	opCode         uint8
	sequenceNumber uint8
	messageCount   uint16
	messages       []frameMessage
}

type worker[T message.Serializable] struct {
	tel *internal.Telemetry
}

func (w *worker[T]) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *worker[T]) Init(_ context.Context, _ any) error {
	return nil
}

func (w *worker[T]) Handle(ctx context.Context, msgIn T) (*Message, error) {
	ctx, span := w.tel.NewTrace(ctx, "process cannelloni frame")
	defer span.End()

	f, err := w.decodeFrame(msgIn.GetBytes())
	if err != nil {
		return nil, err
	}

	res := newMessage()

	res.seqNum = f.sequenceNumber
	res.SetReceiveTime(msgIn.GetReceiveTime())

	messageCount := len(f.messages)
	res.MessageCount = messageCount
	if messageCount > defaultCANMessageNum {
		res.Messages = make([]can.RawMessage, messageCount)
	}

	for idx, tmpMsg := range f.messages {
		res.Messages[idx] = can.RawMessage{
			CANID:   tmpMsg.canID,
			DataLen: int(tmpMsg.dataLen),
			RawData: tmpMsg.data,
		}
	}

	span.SetAttributes(attribute.Int("message_count", messageCount))

	return res, nil
}

func (w *worker[T]) Close(_ context.Context) error {
	return nil
}

func (w *worker[T]) decodeFrame(buf []byte) (*frame, error) {
	if buf == nil {
		return nil, errors.New("nil buffer")
	}

	if len(buf) < 5 {
		return nil, errors.New("not enough data")
	}

	f := frame{
		version:        buf[0],
		opCode:         buf[1],
		sequenceNumber: buf[2],
		messageCount:   binary.BigEndian.Uint16(buf[3:5]),
	}

	f.messages = make([]frameMessage, f.messageCount)
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

func (w *worker[T]) decodeFrameMessage(buf []byte, msg *frameMessage) (int, error) {
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
