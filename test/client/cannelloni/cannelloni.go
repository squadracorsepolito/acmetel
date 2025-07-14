package cannelloni

import (
	"encoding/binary"
	"errors"
)

type FrameMessage struct {
	CANID      uint32
	DataLen    uint8
	CANFDFlags uint8
	Data       []byte
}

func NewFrameMessage(canID uint32, data []byte) *FrameMessage {
	return &FrameMessage{
		CANID:   canID,
		DataLen: uint8(len(data)),
		Data:    data,
	}
}

type Frame struct {
	Version        uint8
	OPCode         uint8
	SequenceNumber uint8
	MessageCount   uint16
	Messages       []*FrameMessage
}

func NewFrame(sequenceNumber uint8, messageCount uint16) *Frame {
	return &Frame{
		Version:        1,
		OPCode:         0,
		SequenceNumber: sequenceNumber,
		MessageCount:   messageCount,
	}
}

func DecodeFrame(buf []byte) (*Frame, error) {
	if buf == nil {
		return nil, errors.New("nil buffer")
	}

	if len(buf) < 5 {
		return nil, errors.New("not enough data")
	}

	f := new(Frame)

	f.Version = buf[0]
	f.OPCode = buf[1]
	f.SequenceNumber = buf[2]
	f.MessageCount = binary.BigEndian.Uint16(buf[3:5])

	f.Messages = make([]*FrameMessage, f.MessageCount)
	pos := 5
	for i := uint16(0); i < f.MessageCount; i++ {
		msg, n, err := decodeFrameMessage(buf[pos:])
		if err != nil {
			return nil, err
		}

		f.Messages[i] = msg
		pos += n
	}

	return f, nil
}

func decodeFrameMessage(buf []byte) (*FrameMessage, int, error) {
	if len(buf) < 5 {
		return nil, 0, errors.New("not enough data")
	}

	msg := new(FrameMessage)

	n := 5

	msg.CANID = binary.BigEndian.Uint32(buf[0:4])

	isCANFD := false
	tmpDataLen := buf[4]
	if tmpDataLen|0x80 == 0x80 {
		isCANFD = true
	}

	if isCANFD {
		if len(buf) < 6 {
			return nil, 0, errors.New("not enough data")
		}

		msg.DataLen = tmpDataLen & 0x7f
		msg.CANFDFlags = buf[5]
		n++
	} else {
		msg.DataLen = tmpDataLen
	}

	msg.Data = make([]byte, msg.DataLen)
	for i := range int(msg.DataLen) {
		msg.Data[i] = buf[n]
		n++
	}

	return msg, n, nil
}

func (f *Frame) Encode() []byte {
	buf := make([]byte, 5)

	buf[0] = f.Version
	buf[1] = f.OPCode
	buf[2] = f.SequenceNumber
	binary.BigEndian.PutUint16(buf[3:5], f.MessageCount)

	for _, msg := range f.Messages {
		msgBuf := msg.Encode()
		buf = append(buf, msgBuf...)
	}

	return buf
}

func (f *Frame) AddMessage(msg *FrameMessage) {
	f.Messages = append(f.Messages, msg)
	f.MessageCount++
}

func (cfm *FrameMessage) Encode() []byte {
	isCANFD := cfm.CANFDFlags != 0

	baseSize := 5
	if isCANFD {
		baseSize = 6
	}

	buf := make([]byte, baseSize+len(cfm.Data))

	binary.BigEndian.PutUint32(buf[0:4], cfm.CANID)

	if isCANFD {
		buf[4] = cfm.DataLen | 0x80
		buf[5] = cfm.CANFDFlags
	} else {
		buf[4] = cfm.DataLen
	}

	for i := range int(cfm.DataLen) {
		buf[baseSize+i] = cfm.Data[i]
	}

	return buf
}
