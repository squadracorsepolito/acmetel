package cannelloni

import (
	"encoding/binary"
	"errors"
)

type CannelloniFrameMessage struct {
	CANID      uint32
	DataLen    uint8
	CANFDFlags uint8
	Data       []byte
}

func NewCannelloniFrameMessage(canID uint32, data []byte) *CannelloniFrameMessage {
	return &CannelloniFrameMessage{
		CANID:   canID,
		DataLen: uint8(len(data)),
		Data:    data,
	}
}

type CannelloniFrame struct {
	Version        uint8
	OPCode         uint8
	SequenceNumber uint8
	MessageCount   uint16
	Messages       []*CannelloniFrameMessage
}

func NewCannelloniFrame(sequenceNumber uint8, messageCount uint16) *CannelloniFrame {
	return &CannelloniFrame{
		Version:        1,
		OPCode:         0,
		SequenceNumber: sequenceNumber,
		MessageCount:   messageCount,
	}
}

func DecodeFrame(buf []byte) (*CannelloniFrame, error) {
	if len(buf) < 5 {
		return nil, errors.New("not enough data")
	}

	f := new(CannelloniFrame)

	f.Version = buf[0]
	f.OPCode = buf[1]
	f.SequenceNumber = buf[2]
	f.MessageCount = binary.BigEndian.Uint16(buf[3:5])

	f.Messages = make([]*CannelloniFrameMessage, f.MessageCount)
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

func decodeFrameMessage(buf []byte) (*CannelloniFrameMessage, int, error) {
	if len(buf) < 5 {
		return nil, 0, errors.New("not enough data")
	}

	msg := new(CannelloniFrameMessage)

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

func (f *CannelloniFrame) Encode() []byte {
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

func (f *CannelloniFrame) AddMessage(msg *CannelloniFrameMessage) {
	f.Messages = append(f.Messages, msg)
	f.MessageCount++
}

func (cfm *CannelloniFrameMessage) Encode() []byte {
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
