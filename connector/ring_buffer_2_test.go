package connector

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RingBuffer2(t *testing.T) {
	assert := assert.New(t)

	// capacity := 4096

	// rb := NewRingBuffer2[int](uint32(capacity))

	// capacity = int(rb.capacity)

	// // Test pack and unpack
	// headTail := rb.pack(2048, 1024)
	// head, tail := rb.unpack(headTail)
	// assert.Equal(uint32(2048), head)
	// assert.Equal(uint32(1024), tail)

	// // Test push and pop sequentially
	// for val := range capacity {
	// 	assert.True(rb.push(val))
	// }
	// assert.False(rb.push(capacity))

	// for val := range capacity {
	// 	item, ok := rb.pop()
	// 	assert.True(ok)
	// 	assert.Equal(val, item)
	// }
	// _, ok := rb.pop()
	// assert.False(ok)

	// assert.True(rb.push(capacity))
	// item, ok := rb.pop()
	// assert.True(ok)
	// assert.Equal(capacity, item)

	// Test pop concurrently
	testPushPop(t, assert, NewRingBuffer2[int](4096), 100_000, 4, 4)

	// Common test
	testConnector(t, NewRingBuffer2[int](uint32(testSize)), testItemCount)
}

func testPushPop(t *testing.T, assert *assert.Assertions, rb *RingBuffer2[int], itemCount, pushConcurrency, popConcurrency int) {
	var popCount atomic.Uint64

	popWg := &sync.WaitGroup{}
	popWg.Add(popConcurrency)

	readCount := &sync.Map{}
	for val := range itemCount {
		readCount.Store(val, 0)
	}

	var popFalseCount atomic.Uint64

	for range popConcurrency {
		go func() {
			defer popWg.Done()

			for {
				if popCount.Load() == uint64(itemCount) {
					break
				}

				item, ok := rb.pop()
				if !ok {
					popFalseCount.Add(1)
					continue
				}
				popCount.Add(1)

				prev, ok := readCount.Load(item)
				readCount.Store(item, prev.(int)+1)
			}
		}()
	}

	pushWg := &sync.WaitGroup{}
	pushWg.Add(pushConcurrency)

	pushTargetItems := itemCount / pushConcurrency

	for idx := range pushConcurrency {
		go func(idx int) {
			defer pushWg.Done()

			offset := idx * pushTargetItems
			val := 0
			failCount := 0

			for val < pushTargetItems {
				if !rb.push(val + offset) {
					failCount++
					continue
				}
				val++
			}

			t.Logf("push -> idx: %d; fails: %d", idx, failCount)
		}(idx)
	}

	pushWg.Wait()
	popWg.Wait()

	assert.Equal(uint64(itemCount), popCount.Load())

	effectiveItemCount := 0
	readCount.Range(func(_, value any) bool {
		assert.Equal(1, value)
		effectiveItemCount++
		return true
	})

	assert.Equal(itemCount, effectiveItemCount)

	t.Logf("pop false count: %d", popFalseCount.Load())
}
