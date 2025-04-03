package acmetel

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
)

var (
	// ErrBufferFull is returned when a write operation fails because the buffer is full
	ErrBufferFull = errors.New("ring buffer: buffer is full")
	// ErrClosed is returned when operations are attempted on a closed buffer
	ErrClosed = errors.New("ring buffer: buffer is closed")
	// ErrBufferEmpty is returned when a non-blocking read operation finds no data
	ErrBufferEmpty = errors.New("ring buffer: buffer is empty")
)

// RingBuffer is a generic lock-free ring buffer implementation
type RingBuffer[T any] struct {
	buffer   []T
	mask     uint64
	readIdx  atomic.Uint64 // index for reading from the buffer
	writeIdx atomic.Uint64 // index for writing to the buffer
	size     uint64
	closed   atomic.Bool
	notEmpty *sync.Cond
	notFull  *sync.Cond
	mu       sync.Mutex // Mutex for conditions

	isFull  atomic.Bool
	isEmpty atomic.Bool
}

// New creates a new RingBuffer with the given capacity.
// The capacity will be rounded up to the next power of 2.
func NewRingBuffer[T any](capacity uint64) *RingBuffer[T] {
	// Round capacity to the next power of 2
	capacity--
	capacity |= capacity >> 1
	capacity |= capacity >> 2
	capacity |= capacity >> 4
	capacity |= capacity >> 8
	capacity |= capacity >> 16
	capacity |= capacity >> 32
	capacity++

	rb := &RingBuffer[T]{
		buffer: make([]T, capacity),
		mask:   capacity - 1,
		size:   capacity,
	}

	// Initialize condition variables
	rb.notEmpty = sync.NewCond(&rb.mu)
	rb.notFull = sync.NewCond(&rb.mu)

	return rb
}

// Write adds an item to the buffer.
// It returns ErrBufferFull if the buffer is full and no space becomes available.
// This method blocks until space is available or the buffer is closed.
func (rb *RingBuffer[T]) Write(item T) error {
	for {
		if rb.closed.Load() {
			return ErrClosed
		}

		readIdx := rb.readIdx.Load()
		writeIdx := rb.writeIdx.Load()

		// Check if buffer is full
		if writeIdx-readIdx >= rb.size {

			rb.isFull.Store(true)

			// Buffer is full, wait for space
			rb.mu.Lock()
			if rb.closed.Load() {
				rb.mu.Unlock()
				rb.isFull.Store(false)
				return ErrClosed
			}

			rb.notFull.Wait()

			rb.mu.Unlock()
			rb.isFull.Store(false)

			continue
		}

		// Try to write at the current writeIdx position
		if rb.writeIdx.CompareAndSwap(writeIdx, writeIdx+1) {
			// We got the slot, write the item
			rb.buffer[writeIdx&rb.mask] = item

			// Signal that buffer is not empty

			if rb.isEmpty.Load() {
				rb.mu.Lock()
				rb.notEmpty.Signal()
				rb.mu.Unlock()
			}

			return nil
		}

		// CAS failed, another writer got here first - yield and retry
		runtime.Gosched()
	}
}

// Read retrieves and removes an item from the buffer.
// It blocks until an item is available or the buffer is closed.
func (rb *RingBuffer[T]) Read() (T, error) {
	var zero T

	for {
		readIdx := rb.readIdx.Load()
		writeIdx := rb.writeIdx.Load()

		// Check if buffer is empty
		if readIdx >= writeIdx {
			if rb.closed.Load() {
				return zero, ErrClosed
			}

			rb.isEmpty.Store(true)

			// Buffer is empty, wait for data
			rb.mu.Lock()
			if rb.closed.Load() {
				rb.mu.Unlock()
				rb.isEmpty.Store(false)
				return zero, ErrClosed
			}

			rb.notEmpty.Wait()

			rb.mu.Unlock()

			rb.isEmpty.Store(false)

			continue
		}

		// Try to claim the item at the current readIdx position
		if rb.readIdx.CompareAndSwap(readIdx, readIdx+1) {
			// We got the slot, read the item
			item := rb.buffer[readIdx&rb.mask]

			// Signal that buffer is not full

			if rb.isFull.Load() {
				rb.mu.Lock()
				rb.notFull.Signal()
				rb.mu.Unlock()
			}

			return item, nil
		}

		// CAS failed, another reader got here first - yield and retry
		runtime.Gosched()
	}
}

// Close marks the buffer as closed. No further writes will be accepted.
// Readers can still drain the buffer.
func (rb *RingBuffer[T]) Close() {
	if !rb.closed.CompareAndSwap(false, true) {
		return
	}

	// Signal waiting goroutines to check the closed flag
	rb.mu.Lock()
	rb.notEmpty.Broadcast()
	rb.notFull.Broadcast()
	rb.mu.Unlock()
}

// Len returns the current number of items in the buffer
func (rb *RingBuffer[T]) Len() uint64 {
	readIdx := rb.readIdx.Load()
	writeIdx := rb.writeIdx.Load()
	return writeIdx - readIdx
}

// Cap returns the capacity of the buffer
func (rb *RingBuffer[T]) Cap() uint64 {
	return rb.size
}
