package connector

import (
	"github.com/squadracorsepolito/acmetel/internal/rb"
)

type RingBuffer[T any] = rb.RingBuffer[T]

var ErrClosed = rb.ErrClosed
var ErrReadTimeout = rb.ErrReadTimeout

func NewRingBuffer[T any](capacity uint32) *RingBuffer[T] {
	return rb.NewRingBuffer[T](capacity, rb.BufferKindSPSC)
}
