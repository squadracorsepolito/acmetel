package rb

import (
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/cpu"
)

type spscBuffer[T any] struct {
	head uint32

	_ cpu.CacheLinePad

	tail uint32

	_ cpu.CacheLinePad

	// headShared atomic.Uint32

	headPtr unsafe.Pointer

	_ cpu.CacheLinePad

	tailPtr unsafe.Pointer

	// tailShared atomic.Uint32

	_ cpu.CacheLinePad

	capacity uint32
	capMask  uint32

	_ cpu.CacheLinePad

	buffer []T
}

func newSPSCBuffer[T any](capacity uint32) *spscBuffer[T] {
	b := &spscBuffer[T]{
		capacity: capacity,
		capMask:  capacity - 1,

		buffer: make([]T, capacity),
	}

	b.headPtr = unsafe.Pointer(&b.head)
	b.tailPtr = unsafe.Pointer(&b.tail)

	return b
}

func (b *spscBuffer[T]) push(item T) bool {
	// Get head and tail
	head := b.head
	tail := atomic.LoadUint32((*uint32)(b.tailPtr))

	// Check if buffer is full
	if head-tail >= b.capacity {
		// Buffer is full
		return false
	}

	// Add the item to the buffer
	itemIndex := head & b.capMask
	b.buffer[itemIndex] = item

	// Increase head
	b.head = head + 1
	atomic.StoreUint32((*uint32)(b.headPtr), head+1)

	return true
}

func (b *spscBuffer[T]) pop() (T, bool) {
	var zero T

	// Get head and tail
	head := atomic.LoadUint32((*uint32)(b.headPtr))
	tail := b.tail

	// Check if buffer is empty
	if head == tail {
		// Buffer is empty
		return zero, false
	}

	// Get the item
	itemIndex := tail & b.capMask
	item := b.buffer[itemIndex]

	// Increase tail
	b.tail = tail + 1
	atomic.StoreUint32((*uint32)(b.tailPtr), tail+1)

	return item, true
}

func (b *spscBuffer[T]) len() uint32 {
	head := atomic.LoadUint32((*uint32)(b.headPtr))
	tail := atomic.LoadUint32((*uint32)(b.tailPtr))

	// Check if tail has wrapped around
	if head < tail {
		return head + b.capacity - tail
	}

	return head - tail
}

type spsc2Buffer[T any] struct {
	head uint32

	_ cpu.CacheLinePad

	tail uint32

	_ cpu.CacheLinePad

	headShared atomic.Uint32

	_ cpu.CacheLinePad

	tailShared atomic.Uint32

	_ cpu.CacheLinePad

	capacity uint32
	capMask  uint32

	_ cpu.CacheLinePad

	buffer []T
}

func newSPSC2Buffer[T any](capacity uint32) *spsc2Buffer[T] {
	return &spsc2Buffer[T]{
		capacity: capacity,
		capMask:  capacity - 1,

		buffer: make([]T, capacity),
	}
}

func (b *spsc2Buffer[T]) push(item T) bool {
	// Get head and tail
	head := b.head
	tail := b.tailShared.Load()

	// Check if buffer is full
	if head-tail >= b.capacity {
		// Buffer is full
		return false
	}

	// Add the item to the buffer
	itemIndex := head & b.capMask
	b.buffer[itemIndex] = item

	// Increase head
	b.head = head + 1
	b.headShared.Store(head + 1)

	return true
}

func (b *spsc2Buffer[T]) pop() (T, bool) {
	var zero T

	// Get head and tail
	head := b.headShared.Load()
	tail := b.tail

	// Check if buffer is empty
	if head == tail {
		// Buffer is empty
		return zero, false
	}

	// Get the item
	itemIndex := tail & b.capMask
	item := b.buffer[itemIndex]

	// Increase tail
	b.tail = tail + 1
	b.tailShared.Store(tail + 1)

	return item, true
}

func (b *spsc2Buffer[T]) len() uint32 {
	head := b.headShared.Load()
	tail := b.tailShared.Load()

	// Check if tail has wrapped around
	if head < tail {
		return head + b.capacity - tail
	}

	return head - tail
}
