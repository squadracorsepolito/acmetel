package internal

import (
	"context"
	"sync"
	"sync/atomic"
)

type RingBuffer[T any] struct {
	buf []T

	size uint64
	mask uint64

	writeIdx atomic.Uint64
	readIdx  atomic.Uint64

	isFull  atomic.Bool
	isEmpty atomic.Bool

	writeCondMux *sync.Mutex
	writeCond    *sync.Cond

	readCondMux *sync.Mutex
	readCond    *sync.Cond
}

func NewRingBuffer[T any](size uint64) *RingBuffer[T] {
	buf := make([]T, size)

	writeCondMux := &sync.Mutex{}
	readCondMux := &sync.Mutex{}

	return &RingBuffer[T]{
		buf: buf,

		size: size,
		mask: size - 1,

		writeCond:    sync.NewCond(writeCondMux),
		writeCondMux: writeCondMux,

		readCond:    sync.NewCond(readCondMux),
		readCondMux: readCondMux,
	}
}

func (rb *RingBuffer[T]) Init(ctx context.Context) {
	go func() {
		<-ctx.Done()
		rb.writeCond.Broadcast()
		rb.readCond.Broadcast()
	}()
}

func (rb *RingBuffer[T]) Write(ctx context.Context, item T) error {
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

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			rb.isFull.Store(true)

			rb.writeCondMux.Lock()
			rb.writeCond.Wait()
			rb.writeCondMux.Unlock()

			rb.isFull.Store(false)

			continue
		}

		if rb.writeIdx.CompareAndSwap(writeIdx, nextWriteIdx) {
			break
		}
	}

	rb.buf[writeIdx&rb.mask] = item

	rb.wakeupReader()

	return nil
}

func (rb *RingBuffer[T]) Read(ctx context.Context) (T, error) {
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

			select {
			case <-ctx.Done():
				return *new(T), ctx.Err()
			default:
			}

			rb.isEmpty.Store(true)

			rb.readCondMux.Lock()
			rb.readCond.Wait()
			rb.readCondMux.Unlock()

			rb.isEmpty.Store(false)

			continue
		}

		item := rb.buf[readIdx&rb.mask]

		if rb.readIdx.CompareAndSwap(readIdx, (readIdx+1)&rb.mask) {
			rb.wakeupWriter()

			return item, nil
		}
	}
}

func (rb *RingBuffer[T]) wakeupWriter() {
	if rb.isFull.Load() {
		rb.writeCond.Signal()
	}
}

func (rb *RingBuffer[T]) wakeupReader() {
	if rb.isEmpty.Load() {
		rb.readCond.Signal()
	}
}

func (rb *RingBuffer[T]) Size() uint64 {
	return (rb.writeIdx.Load() - rb.readIdx.Load()) & rb.mask
}

func (rb *RingBuffer[T]) Capacity() uint64 {
	return rb.size
}

func (rb *RingBuffer[T]) IsEmpty() bool {
	return rb.isEmpty.Load()
}

func (rb *RingBuffer[T]) IsFull() bool {
	return rb.isFull.Load()
}
