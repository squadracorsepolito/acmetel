package connector

import (
	"github.com/squadracorsepolito/acmetel/internal/message"
	"github.com/squadracorsepolito/acmetel/internal/rb"
)

type msgVal = message.Envelope

type msgWrap[T msgVal] = message.Message[T]

// RingBuffer is a lock-free spsc generic ring buffer.
type RingBuffer[T any] = rb.RingBuffer[T]
