package rb

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/cpu"
)

var ErrClosed = errors.New("ring buffer: buffer is closed")

type buffer[T any] interface {
	push(T) bool
	pop() (T, bool)
	len() uint32
}

type BufferKind uint8

const (
	BufferKindSPSC BufferKind = iota
	BufferKindSPSC2
)

func (bk BufferKind) String() string {
	switch bk {
	case BufferKindSPSC:
		return "SPSC"
	case BufferKindSPSC2:
		return "SPSC2"
	default:
		return "unknown"
	}
}

type RingBuffer[T any] struct {
	kind BufferKind

	spsc  *spscBuffer[T]
	spsc2 *spsc2Buffer[T]

	_ cpu.CacheLinePad

	// isClosed states whether the buffer is closed.
	isClosed atomic.Bool

	_ cpu.CacheLinePad

	// isFull states whether the buffer is full.
	isFull atomic.Bool

	_ cpu.CacheLinePad

	// isEmpty states whether the buffer is empty.
	isEmpty atomic.Bool

	_ cpu.CacheLinePad

	// notEmpty and notFull are used to signal that the buffer is not empty or full
	notEmpty *sync.Cond
	notFull  *sync.Cond
	mux      *sync.Mutex
}

func NewRingBuffer[T any](capacity uint32, kind BufferKind) *RingBuffer[T] {
	mux := &sync.Mutex{}

	rb := &RingBuffer[T]{
		kind: kind,

		mux:      mux,
		notEmpty: sync.NewCond(mux),
		notFull:  sync.NewCond(mux),
	}

	parsedCapacity := roundToPowerOf2(capacity)

	switch kind {
	case BufferKindSPSC:
		rb.spsc = newSPSCBuffer[T](parsedCapacity)
	case BufferKindSPSC2:
		rb.spsc2 = newSPSC2Buffer[T](parsedCapacity)
	}

	return rb
}

func (rb *RingBuffer[T]) push(item T) bool {
	switch rb.kind {
	case BufferKindSPSC:
		return rb.spsc.push(item)
	case BufferKindSPSC2:
		return rb.spsc2.push(item)
	default:
		return false
	}
}

func (rb *RingBuffer[T]) pop() (T, bool) {
	switch rb.kind {
	case BufferKindSPSC:
		return rb.spsc.pop()
	case BufferKindSPSC2:
		return rb.spsc2.pop()
	default:
		return *new(T), false
	}
}

func (rb *RingBuffer[T]) len() uint32 {
	switch rb.kind {
	case BufferKindSPSC:
		return rb.spsc.len()
	case BufferKindSPSC2:
		return rb.spsc2.len()
	default:
		return 0
	}
}

func (rb *RingBuffer[T]) Write(item T) error {
	// Check if buffer is closed
	if rb.isClosed.Load() {
		return ErrClosed
	}

	// Try to push the item
	if rb.push(item) {
		goto cleanup
	}

	// The buffer is full, yield to other goroutines
	runtime.Gosched()

	// Try to push the item
	for !rb.push(item) {
		// Buffer is still full, yield to other goroutines
		runtime.Gosched()

		// Retry to push the item
		if rb.push(item) {
			goto cleanup
		}

		// Buffer is full, wait for space
		rb.mux.Lock()

		// Set buffer as full
		rb.isFull.Store(true)

		// Check if buffer is closed
		if rb.isClosed.Load() {
			rb.mux.Unlock()
			return ErrClosed
		}

		// Wait for space
		rb.notFull.Wait()

		// Someone signaled the buffer as not full
		rb.mux.Unlock()
	}

cleanup:
	// Check if buffer is marked as empty
	if rb.isEmpty.Load() {
		rb.mux.Lock()

		// Signal buffer as not empty to other goroutines
		rb.notEmpty.Broadcast()

		// Set buffer as not empty.
		rb.isEmpty.Store(false)

		rb.mux.Unlock()
	}

	return nil
}

func (rb *RingBuffer[T]) Read() (T, error) {
	var item T
	var popOk bool

	// Try to pop an item
	item, popOk = rb.pop()
	if popOk {
		goto cleanup
	}

	// The buffer is empty, yield to other goroutines
	runtime.Gosched()

	// Try to pop an item
	for {
		item, popOk = rb.pop()
		if popOk {
			goto cleanup
		}

		// Buffer is still empty, yield to other goroutines
		runtime.Gosched()

		// Retry to pop an item
		item, popOk = rb.pop()
		if popOk {
			goto cleanup
		}

		// Buffer is empty, wait for data
		rb.mux.Lock()

		// Set buffer as empty
		rb.isEmpty.Store(true)

		// Check if buffer is closed
		if rb.isClosed.Load() {
			rb.mux.Unlock()
			return item, ErrClosed
		}

		// Wait for data
		rb.notEmpty.Wait()

		// Someone signaled the buffer as not empty
		rb.mux.Unlock()
	}

cleanup:
	// Check if buffer is marked as full
	if rb.isFull.Load() {
		rb.mux.Lock()

		// Signal buffer as not full to other goroutines
		rb.notFull.Broadcast()

		// Set buffer as not full
		rb.isFull.Store(false)

		rb.mux.Unlock()
	}

	return item, nil
}

func (rb *RingBuffer[T]) Len() uint32 {
	return rb.len()
}
