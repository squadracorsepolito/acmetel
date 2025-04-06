package connector

import (
	"runtime"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/cpu"
)

const ptrSize = 32

type slot[T any] struct {
	//busy atomic.Bool
	dataReady atomic.Bool
	data      T
}

type RingBuffer2[T any] struct {
	headTail atomic.Uint64

	_      cpu.CacheLinePad
	closed atomic.Bool

	_      cpu.CacheLinePad
	isFull atomic.Bool

	_       cpu.CacheLinePad
	isEmpty atomic.Bool

	capacity uint32
	capMask  uint32

	mux      *sync.Mutex
	notEmpty *sync.Cond
	notFull  *sync.Cond

	buffer []slot[T]
}

func NewRingBuffer2[T any](capacity uint32) *RingBuffer2[T] {
	capacity--
	capacity |= capacity >> 1
	capacity |= capacity >> 2
	capacity |= capacity >> 4
	capacity |= capacity >> 8
	capacity |= capacity >> 16
	capacity++

	mux := &sync.Mutex{}

	return &RingBuffer2[T]{
		capacity: capacity,
		capMask:  capacity - 1,

		buffer: make([]slot[T], capacity),

		mux:      mux,
		notEmpty: sync.NewCond(mux),
		notFull:  sync.NewCond(mux),
	}
}

func (rb *RingBuffer2[T]) pack(head, tail uint32) uint64 {
	const mask = 1<<ptrSize - 1
	return (uint64(head)<<ptrSize | uint64(tail&mask))
}

func (rb *RingBuffer2[T]) unpack(headTail uint64) (head, tail uint32) {
	const mask = 1<<ptrSize - 1
	head = uint32((headTail >> ptrSize) & mask)
	tail = uint32(headTail & mask)
	return
}

// func (rb *RingBuffer2[T]) push(item T) bool {
// 	for {
// 		headTail := rb.headTail.Load()
// 		head, tail := rb.unpack(headTail)

// 		if head-tail >= rb.capacity {
// 			// Buffer is full
// 			return false
// 		}

// 		newHeadTail := rb.pack(head+1, tail)
// 		if !rb.headTail.CompareAndSwap(headTail, newHeadTail) {
// 			continue
// 		}

// 		slot := &rb.buffer[head&rb.capMask]

// 		for !slot.busy.CompareAndSwap(false, true) {
// 			// Slot is not clean
// 			runtime.Gosched()
// 		}

// 		slot.data = item

// 		return true
// 	}
// }

// func (rb *RingBuffer2[T]) pop() (T, bool) {
// 	for {
// 		headTail := rb.headTail.Load()
// 		head, tail := rb.unpack(headTail)

// 		if head == tail {
// 			// Buffer is empty
// 			return *new(T), false
// 		}

// 		nextHeadTail := rb.pack(head, tail+1)
// 		if !rb.headTail.CompareAndSwap(headTail, nextHeadTail) {
// 			continue
// 		}

// 		slot := &rb.buffer[tail&rb.capMask]

// 		item := slot.data

// 		slot.busy.Store(false)

// 		return item, true
// 	}
// }

func (rb *RingBuffer2[T]) push(item T) bool {
	for {
		headTail := rb.headTail.Load()
		head, tail := rb.unpack(headTail)

		if head-tail >= rb.capacity {
			// Buffer is full
			return false
		}

		// Get slot and check it's not still being read
		slotIndex := head & rb.capMask
		slot := &rb.buffer[slotIndex]

		// If dataReady is true, it means this slot hasn't been consumed yet
		// Wait until the slot is marked as consumed
		if slot.dataReady.Load() {
			runtime.Gosched()
			continue
		}

		// Claim this slot by advancing head pointer
		newHeadTail := rb.pack(head+1, tail)
		if !rb.headTail.CompareAndSwap(headTail, newHeadTail) {
			// Someone else modified the buffer, retry
			continue
		}

		// Write the data
		slot.data = item

		// Mark data as ready AFTER writing
		// This is the memory barrier consumers will check
		slot.dataReady.Store(true)

		return true
	}
}

func (rb *RingBuffer2[T]) pop() (T, bool) {
	for {
		headTail := rb.headTail.Load()
		head, tail := rb.unpack(headTail)

		if head == tail {
			// Buffer is empty
			return *new(T), false
		}

		// Get slot
		slotIndex := tail & rb.capMask
		slot := &rb.buffer[slotIndex]

		// Check if data is actually ready to be read
		// This is crucial - we must verify the producer has finished writing
		if !slot.dataReady.Load() {
			// Data not yet ready, try again
			runtime.Gosched()
			continue
		}

		// Try to claim this slot for reading by advancing tail
		nextHeadTail := rb.pack(head, tail+1)
		if !rb.headTail.CompareAndSwap(headTail, nextHeadTail) {
			// Someone else modified the buffer, retry
			continue
		}

		// Read the item
		item := slot.data

		// Mark slot as available for reuse
		slot.dataReady.Store(false)

		return item, true
	}
}

func (rb *RingBuffer2[T]) Write(item T) error {
	if rb.closed.Load() {
		return ErrClosed
	}

	for !rb.push(item) {
		// Buffer is full

		// Yield to other goroutines
		runtime.Gosched()

		// Retry another time
		if rb.push(item) {
			break
		}

		rb.isFull.Store(true)

		// It is still full, wait for space
		rb.mux.Lock()

		if rb.closed.Load() {
			rb.mux.Unlock()
			return ErrClosed
		}

		rb.notFull.Wait()

		rb.mux.Unlock()

		rb.isFull.Store(false)
	}

	// Success
	if rb.isEmpty.Load() {
		rb.mux.Lock()
		rb.notEmpty.Signal()
		rb.mux.Unlock()
	}

	return nil
}

func (rb *RingBuffer2[T]) Read() (T, error) {
	var item T

	for {
		tmpItem, ok := rb.pop()
		if ok {
			item = tmpItem
			break
		}

		runtime.Gosched()

		tmpItem, ok = rb.pop()
		if ok {
			item = tmpItem
			break
		}

		rb.isEmpty.Store(true)

		rb.mux.Lock()

		if rb.closed.Load() {
			rb.mux.Unlock()
			return item, ErrClosed
		}

		rb.notEmpty.Wait()

		rb.mux.Unlock()

		rb.isEmpty.Store(false)
	}

	// Success

	if rb.isFull.Load() {
		rb.mux.Lock()
		rb.notFull.Signal()
		rb.mux.Unlock()
	}

	return item, nil
}

func (rb *RingBuffer2[T]) Close() {
	if !rb.closed.CompareAndSwap(false, true) {
		return
	}

	rb.mux.Lock()
	rb.notEmpty.Broadcast()
	rb.notFull.Broadcast()
	rb.mux.Unlock()
}
