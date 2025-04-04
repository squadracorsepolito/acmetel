package connector

import (
	"runtime"
	"sync"
	"sync/atomic"
)

const ptrSize = 32

type slot[T any] struct {
	busy atomic.Bool
	data T
}

type RingBuffer2[T any] struct {
	headTail atomic.Uint64

	closed atomic.Bool

	isFull atomic.Bool

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

func (rb *RingBuffer2[T]) push(item T) bool {
	head, tail := rb.unpack(rb.headTail.Load())

	if head-tail >= rb.capacity {
		// Buffer is full
		return false
	}

	slot := &rb.buffer[head&rb.capMask]

	if !slot.busy.CompareAndSwap(false, true) {
		// Slot is not clean
		return false
	}

	slot.data = item

	rb.headTail.Add(1 << ptrSize)

	return true
}

func (rb *RingBuffer2[T]) pop() (T, bool) {
	var zero T
	var slot *slot[T]

	for {
		headTail := rb.headTail.Load()
		head, tail := rb.unpack(headTail)

		if head == tail {
			// Buffer is empty
			return zero, false
		}

		nextHeadTail := rb.pack(head, tail+1)
		if rb.headTail.CompareAndSwap(headTail, nextHeadTail) {
			slot = &rb.buffer[tail&rb.capMask]
			break
		}
	}

	item := slot.data

	slot.busy.Store(false)

	return item, true
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
