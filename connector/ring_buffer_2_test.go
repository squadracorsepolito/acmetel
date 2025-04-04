package connector

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RingBuffer2(t *testing.T) {
	assert := assert.New(t)

	capacity := 1_000_000

	rb := NewRingBuffer2[int](uint32(capacity))

	capacity = int(rb.capacity)

	// Test pack and unpack
	headTail := rb.pack(2048, 1024)
	head, tail := rb.unpack(headTail)
	assert.Equal(uint32(2048), head)
	assert.Equal(uint32(1024), tail)

	// Test push and pop sequentially
	for val := range capacity {
		assert.True(rb.push(val))
	}
	assert.False(rb.push(capacity))

	for val := range capacity {
		item, ok := rb.pop()
		assert.True(ok)
		assert.Equal(val, item)
	}
	_, ok := rb.pop()
	assert.False(ok)

	assert.True(rb.push(capacity))
	item, ok := rb.pop()
	assert.True(ok)
	assert.Equal(capacity, item)

	// Test pop concurrently
	concurrency := 2

	var popCount atomic.Uint64

	popWg := &sync.WaitGroup{}
	popWg.Add(concurrency)

	readCount := &sync.Map{}
	for val := range capacity {
		readCount.Store(val, 0)
	}

	pushWg := &sync.WaitGroup{}
	pushWg.Add(1)

	for range concurrency {
		go func() {
			defer popWg.Done()

			pushWg.Wait()

			for {
				item, ok := rb.pop()
				if !ok {
					break
				}
				popCount.Add(1)

				prev, ok := readCount.Load(item)
				readCount.Store(item, prev.(int)+1)
			}
		}()
	}

	for val := range capacity {
		assert.True(rb.push(val))
	}

	pushWg.Done()

	popWg.Wait()

	assert.Equal(uint64(capacity), popCount.Load())

	readCount.Range(func(_, value any) bool {
		assert.Equal(1, value)
		return true
	})

	// Common test
	testConnector(t, NewRingBuffer2[int](uint32(testSize)), testItemCount)
}
