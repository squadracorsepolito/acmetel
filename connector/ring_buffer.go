package connector

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/cpu"
)

var ErrClosed = errors.New("ring buffer: buffer is closed")

type slot[T any] struct {
	dataReady atomic.Bool
	data      T
}

type RingBuffer[T any] struct {
	// headTail is a uint64 where the top 32 bits are head and the bottom 32 bits are tail.
	// This allows us to atomically read both head and tail in a single load.
	headTail atomic.Uint64

	// used to avoid false sharing
	_ cpu.CacheLinePad

	// closed is used to indicate that the buffer is closed.
	closed atomic.Bool

	_ cpu.CacheLinePad

	// isFull is used to indicate that the buffer is full.
	isFull atomic.Bool

	_ cpu.CacheLinePad

	// isEmpty is used to indicate that the buffer is empty.
	isEmpty atomic.Bool

	_ cpu.CacheLinePad

	capacity uint32
	capMask  uint32

	// notEmpty and notFull are used to signal that the buffer is not empty or full
	notEmpty *sync.Cond
	notFull  *sync.Cond
	mux      *sync.Mutex

	// buffer is a ring buffer of slots
	buffer []slot[T]
}

func NewRingBuffer[T any](capacity uint32) *RingBuffer[T] {
	capacity--
	capacity |= capacity >> 1
	capacity |= capacity >> 2
	capacity |= capacity >> 4
	capacity |= capacity >> 8
	capacity |= capacity >> 16
	capacity++

	mux := &sync.Mutex{}

	return &RingBuffer[T]{
		capacity: capacity,
		capMask:  capacity - 1,

		buffer: make([]slot[T], capacity),

		mux:      mux,
		notEmpty: sync.NewCond(mux),
		notFull:  sync.NewCond(mux),
	}
}

func (rb *RingBuffer[T]) pack(head, tail uint32) uint64 {
	const mask = 1<<32 - 1
	return (uint64(head)<<32 | uint64(tail&mask))
}

func (rb *RingBuffer[T]) unpack(headTail uint64) (head, tail uint32) {
	const mask = 1<<32 - 1
	head = uint32((headTail >> 32) & mask)
	tail = uint32(headTail & mask)
	return
}

func (rb *RingBuffer[T]) push(item T) bool {
	for {
		// Load head and tail
		headTail := rb.headTail.Load()
		head, tail := rb.unpack(headTail)

		// Check if buffer is full
		if head-tail >= rb.capacity {
			// Buffer is full
			return false
		}

		// Get slot and check it's not still being read
		slotIndex := head & rb.capMask
		slot := &rb.buffer[slotIndex]

		// If dataReady is true, it means this slot hasn't been consumed yet
		if slot.dataReady.Load() {
			// Someone else is reading this slot, retry
			runtime.Gosched()
			continue
		}

		// Claim this slot by advancing head pointer
		newHeadTail := rb.pack(head+1, tail)
		if !rb.headTail.CompareAndSwap(headTail, newHeadTail) {
			// Someone else modified the buffer, retry
			runtime.Gosched()
			continue
		}

		// Write the data
		slot.data = item

		// Mark data as ready
		slot.dataReady.Store(true)

		return true
	}
}

func (rb *RingBuffer[T]) pop() (T, bool) {
	for {
		// Load head and tail
		headTail := rb.headTail.Load()
		head, tail := rb.unpack(headTail)

		// Check if buffer is empty
		if head == tail {
			// Buffer is empty
			return *new(T), false
		}

		// Get slot
		slotIndex := tail & rb.capMask
		slot := &rb.buffer[slotIndex]

		// Check if data is actually ready to be read
		if !slot.dataReady.Load() {
			// Data not yet ready, retry
			runtime.Gosched()
			continue
		}

		// Try to claim this slot for reading by advancing tail
		nextHeadTail := rb.pack(head, tail+1)
		if !rb.headTail.CompareAndSwap(headTail, nextHeadTail) {
			// Someone else modified the buffer, retry
			runtime.Gosched()
			continue
		}

		// Read the item
		item := slot.data

		// Mark slot as available for reuse
		slot.dataReady.Store(false)

		return item, true
	}
}

// Write adds an item to the [RingBuffer].
// It blocks until the buffer is not full.
//
// Returns [ErrClosed] if the [RingBuffer] is closed.
func (rb *RingBuffer[T]) Write(item T) error {
	// Check if buffer is closed
	if rb.closed.Load() {
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
		if rb.closed.Load() {
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

// Read retrieves an item from the [RingBuffer].
// It blocks until the buffer is not empty.
//
// Returns [ErrClosed] if the [RingBuffer] is closed.
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
		if rb.closed.Load() {
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

// Close marks the [RingBuffer] as closed.
func (rb *RingBuffer[T]) Close() {
	if !rb.closed.CompareAndSwap(false, true) {
		return
	}

	rb.mux.Lock()
	rb.notEmpty.Broadcast()
	rb.notFull.Broadcast()
	rb.mux.Unlock()
}
