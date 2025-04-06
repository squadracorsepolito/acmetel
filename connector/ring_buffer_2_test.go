package connector

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	testPushPop(t, assert, NewRingBuffer2[int](128), 4000, 4, 4)

	// Common test
	// testConnector(t, NewRingBuffer2[int](uint32(testSize)), testItemCount)
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

				readCount.Store(item, true)
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

	missingItems := 0
	for i := range itemCount {
		if _, ok := readCount.Load(i); !ok {
			missingItems++
		}
	}

	if missingItems > 0 {
		t.Errorf("Missing %d items in total", missingItems)
	}

	t.Logf("pop false count: %d", popFalseCount.Load())
}

func Test_RingBuffer2_MultipleProducersConsumers(t *testing.T) {
	const (
		bufferCapacity   = 2048
		numProducers     = 8
		numConsumers     = 8
		itemsPerProducer = 1_000_000
		totalItems       = numProducers * itemsPerProducer
	)

	rb := NewRingBuffer2[int](bufferCapacity)

	testMultipleProducersConsumers(t, rb, numProducers, numConsumers, itemsPerProducer)
}

func testMultipleProducersConsumers(t *testing.T, connector Connector[int], numProducers, numConsumers, itemsPerProducer int) {
	totalItems := numProducers * itemsPerProducer

	// Used to track received items
	var receivedItems sync.Map
	var receivedCount atomic.Uint64

	// WaitGroups to coordinate test completion
	var producerWg sync.WaitGroup
	var consumerWg sync.WaitGroup

	// Start time for performance measurement
	startTime := time.Now()

	// Start consumers
	consumerWg.Add(numConsumers)
	for i := range numConsumers {
		go func(consumerID int) {
			defer consumerWg.Done()

			// Each consumer reads until the buffer is closed
			for {
				item, err := connector.Read()
				if err != nil {
					if !errors.Is(err, ErrClosed) {
						t.Errorf("Consumer %d received unexpected error: %v", consumerID, err)
					}
					return
				}

				// Mark this item as received
				receivedItems.Store(item, true)
				receivedCount.Add(1)
			}
		}(i)
	}

	// Start producers
	producerWg.Add(numProducers)
	for i := range numProducers {
		go func(producerID int) {
			defer producerWg.Done()

			base := producerID * itemsPerProducer
			for j := 0; j < itemsPerProducer; j++ {
				item := base + j
				err := connector.Write(item)
				if err != nil {
					t.Errorf("Producer %d failed to write item %d: %v", producerID, item, err)
					return
				}
			}
		}(i)
	}

	// Wait for all producers to finish
	producerWg.Wait()

	// Close the buffer
	connector.Close()

	// Wait for consumers to process all items or timeout after 5 seconds
	consumeFinished := make(chan struct{})
	go func() {
		consumerWg.Wait()
		close(consumeFinished)
	}()

	select {
	case <-consumeFinished:
		// All items processed successfully
	case <-time.After(5 * time.Second):
		t.Fatalf("Test timed out. Received %d/%d items", receivedCount.Load(), totalItems)
	}

	// Verify all items were received exactly once
	missingItems := 0
	for i := range totalItems {
		if _, ok := receivedItems.Load(i); !ok {
			missingItems++
		}
	}

	if missingItems > 0 {
		t.Errorf("Missing %d items in total", missingItems)
	}

	// Report performance
	duration := time.Since(startTime)
	itemsPerSec := int(float64(totalItems) / duration.Seconds())
	t.Logf("Processed %d items in %v (%d items/sec)", totalItems, duration, itemsPerSec)
}
