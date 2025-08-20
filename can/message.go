package can

import (
	"github.com/squadracorsepolito/acmetel/internal/message"
)

// RawCANMessageCarrier interface defines the common methods
// for all message types that carry CAN messages.
type RawCANMessageCarrier interface {
	message.Message

	// GetRawMessages returns the list of raw CAN messages.
	GetRawMessages() []RawMessage
}

// RawMessage represents a CAN message before decoding.
type RawMessage struct {
	// CANID is the CAN ID of the message.
	CANID uint32

	// RawData is the payload of the CAN message.
	RawData []byte
	// DataLen is the number of bytes of the payload.
	DataLen int
}

// ValueType represents the type of the value of a signal.
type ValueType int

const (
	// ValueTypeFlag defines a value of type flag (boolean).
	ValueTypeFlag ValueType = iota
	// ValueTypeInt defines a value of type integer.
	ValueTypeInt
	// ValueTypeFloat defines a value of type float.
	ValueTypeFloat
	// ValueTypeEnum defines a value of type enum.
	ValueTypeEnum
)

// Signal represents a decoded CAN signal.
type Signal struct {
	// CANID is the CAN ID of the message that contains this signal.
	CANID uint32

	// Name is the name of the signal.
	Name string

	// RawValue is the raw value of the signal.
	RawValue uint64

	// Type is the type of the value of the signal.
	Type ValueType
	// ValueFlag is the value of the signal as a boolean.
	ValueFlag bool
	// ValueInt is the value of the signal as an integer.
	ValueInt int64
	// ValueFloat is the value of the signal as a float.
	ValueFloat float64
	// ValueEnum is the value of the signal as an enum.
	ValueEnum string
}

// Message represents a decoded CAN message.
// It only contains the value of the signals of every message.
type Message struct {
	message.Base

	// Signals is the list of decoded signals.
	Signals []Signal
	// SignalCount is the number of decoded signals.
	SignalCount int
}

func newMessage() *Message {
	return &Message{
		SignalCount: 0,
		Signals:     []Signal{},
	}
}
