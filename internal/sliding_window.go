package internal

import (
	"iter"
	"sync"
)

type slidingWindow[T any] struct {
	capacity uint64
	capMask  uint64
	currSize uint64

	buffer []T
	mux    *sync.RWMutex

	index uint64
}

func newSlidingWindow[T any](capacity uint64) *slidingWindow[T] {
	capacity--
	capacity |= capacity >> 1
	capacity |= capacity >> 2
	capacity |= capacity >> 4
	capacity |= capacity >> 8
	capacity |= capacity >> 16
	capacity |= capacity >> 32
	capacity++

	return &slidingWindow[T]{
		capacity: capacity,
		capMask:  capacity - 1,

		buffer: make([]T, capacity),
		mux:    &sync.RWMutex{},

		index: 0,
	}
}

func (sw *slidingWindow[T]) push(item T) {
	sw.mux.Lock()
	defer sw.mux.Unlock()

	sw.buffer[sw.index&sw.capMask] = item
	sw.index++

	sw.currSize = min(sw.currSize+1, sw.capacity)
}

func (sw *slidingWindow[T]) iterate() iter.Seq[T] {
	return func(yield func(T) bool) {
		sw.mux.RLock()
		defer sw.mux.RUnlock()

		for i := range sw.currSize {
			if !yield(sw.buffer[i]) {
				return
			}
		}
	}
}
