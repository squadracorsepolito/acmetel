package pool

import (
	"context"

	"github.com/squadracorsepolito/acmetel/internal/rb"
)

type fanOut[T any] struct {
	buffer *rb.RingBuffer[T]
}

func newFanOut[T any](bufferCapacity int) *fanOut[T] {
	return &fanOut[T]{
		buffer: rb.NewRingBuffer[T](uint32(bufferCapacity), rb.BufferKindMPMC),
	}
}

func (fo *fanOut[T]) addTask(ctx context.Context, task T) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return fo.buffer.Write(task)
}

func (fo *fanOut[T]) readTask() (T, error) {
	return fo.buffer.Read()
}

func (fo *fanOut[T]) close() {
	fo.buffer.Close()
}
