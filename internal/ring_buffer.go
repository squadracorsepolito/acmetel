package internal

import (
	"context"
	"sync/atomic"
)

type RingBuffer[T any] struct {
	buf []T

	size uint64
	mask uint64

	writeIdx atomic.Uint64
	readIdx  atomic.Uint64
}

func NewRingBuffer[T any](size uint64) *RingBuffer[T] {
	buf := make([]T, size)

	return &RingBuffer[T]{
		buf: buf,

		size: size,
		mask: size - 1,
	}
}

func (rb *RingBuffer[T]) Put(ctx context.Context, item T) error {
	var writeIdx uint64

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		writeIdx = rb.writeIdx.Load()
		readIdx := rb.readIdx.Load()

		nextWriteIdx := (writeIdx + 1) & rb.mask

		if nextWriteIdx == readIdx {
			// buffer is full
			continue
		}

		if rb.writeIdx.CompareAndSwap(writeIdx, nextWriteIdx) {
			break
		}
	}

	rb.buf[writeIdx&rb.mask] = item

	return nil
}

func (rb *RingBuffer[T]) Get(ctx context.Context) (T, error) {
	for {
		select {
		case <-ctx.Done():
			return *new(T), ctx.Err()
		default:
		}

		readIdx := rb.readIdx.Load()
		writeIdx := rb.writeIdx.Load()

		if readIdx == writeIdx {
			// buffer is empty
			continue
		}

		item := rb.buf[readIdx&rb.mask]

		if rb.readIdx.CompareAndSwap(readIdx, (readIdx+1)&rb.mask) {
			return item, nil
		}
	}
}

func (rb *RingBuffer[T]) Size() uint64 {
	return (rb.writeIdx.Load() - rb.readIdx.Load()) & rb.mask
}

func (rb *RingBuffer[T]) Capacity() uint64 {
	return rb.size
}

func (rb *RingBuffer[T]) IsEmpty() bool {
	return rb.readIdx.Load() == rb.writeIdx.Load()
}

func (rb *RingBuffer[T]) IsFull() bool {
	return rb.readIdx.Load() == (rb.writeIdx.Load()+1)&rb.mask
}
