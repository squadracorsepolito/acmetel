package rb

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	itemsCount     = 1_000_000
	bufferCapacity = 128
)

func Test_spscBuffer(t *testing.T) {
	assert := assert.New(t)
	b := newSPSCBuffer[int](bufferCapacity)
	testBuffer(assert, 1, 1, b)
}

type buffer[T any] interface {
	push(item T) bool
	pop() (T, bool)
}

func testBuffer(assert *assert.Assertions, prodNum, consNum int, buffer buffer[int]) {
	pushWg := &sync.WaitGroup{}
	pushWg.Add(prodNum)

	valueMap := &sync.Map{}
	for val := range itemsCount {
		valueMap.Store(val, true)
	}

	itemsPerProducer := itemsCount / prodNum
	for idx := range prodNum {
		go func(idx int) {
			defer pushWg.Done()

			baseVal := idx * itemsPerProducer
			produced := 0
			for {
				if !buffer.push(baseVal + produced) {
					continue
				}

				produced++
				if produced == itemsPerProducer {
					break
				}
			}
		}(idx)
	}

	popWg := &sync.WaitGroup{}
	popWg.Add(consNum)

	var totalConsumed atomic.Int64

	itemsPerConsumer := itemsCount / consNum
	for range consNum {
		go func() {
			defer popWg.Done()

			consumed := 0
			for {
				val, ok := buffer.pop()
				if !ok {
					continue
				}

				assert.True(valueMap.CompareAndSwap(val, true, false))
				totalConsumed.Add(1)

				consumed++
				if consumed == itemsPerConsumer {
					break
				}
			}
		}()
	}

	pushWg.Wait()
	popWg.Wait()

	assert.Equal(int64(itemsCount), totalConsumed.Load())
}

func Test_RingBuffer(t *testing.T) {
	kinds := []BufferKind{BufferKindSPSC}
	capacity := 128
	itemsPerConsumer := 1_000_000

	for _, kind := range kinds {
		t.Run(kind.String(), func(t *testing.T) {
			testRingBuffer(t, kind, capacity, 1, 1, itemsPerConsumer)
		})
	}
}

func testRingBuffer(t *testing.T, kind BufferKind, capacity, numProducers, numConsumers, itemsPerProducer int) {
	assert := assert.New(t)

	totalItems := numProducers * itemsPerProducer

	rb := NewRingBuffer[int](uint32(capacity), kind)

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
	for range numConsumers {
		go func() {
			defer consumerWg.Done()

			// Each consumer reads until the buffer is closed
			for {
				item, err := rb.Read()
				if err != nil {
					assert.ErrorIs(err, ErrClosed)
					return
				}

				// Mark this item as received
				receivedItems.Store(item, true)
				receivedCount.Add(1)
			}
		}()
	}

	// Start producers
	producerWg.Add(numProducers)
	for i := range numProducers {
		go func(producerID int) {
			defer producerWg.Done()

			base := producerID * itemsPerProducer
			for j := range itemsPerProducer {
				item := base + j
				err := rb.Write(item)
				assert.NoError(err)
				if err != nil {
					return
				}
			}
		}(i)
	}

	// Wait for all producers to finish
	producerWg.Wait()

	t.Log("Finished producing")

	// Close the buffer
	rb.Close()

	// Wait for all consumers to finish
	consumerWg.Wait()
	t.Log("Finished consuming")

	// Verify all items were received
	assert.Equal(uint64(totalItems), receivedCount.Load())

	// Verify all items were received exactly once
	missingItems := 0
	for i := range totalItems {
		if _, ok := receivedItems.Load(i); !ok {
			missingItems++
		}
	}
	assert.Zero(missingItems)

	// Report performance
	duration := time.Since(startTime)
	itemsPerSec := int(float64(totalItems) / duration.Seconds())
	t.Logf("Processed %d items in %v (%d items/sec)", totalItems, duration, itemsPerSec)
}

func Benchmark_RingBuffers(b *testing.B) {
	b.ReportAllocs()

	kinds := []BufferKind{BufferKindSPSC}
	capacities := []int{512, 1024, 2048, 4096}
	for _, kind := range kinds {
		kindStr := kind.String()

		for _, capacity := range capacities {
			capacityStr := strconv.Itoa(capacity)

			b.Run("WriteReadCycle-"+kindStr+"-"+capacityStr, func(b *testing.B) {
				benchWriteReadCycle(b, kind, capacity)
			})

			b.Run("WriteReadSteady-"+kindStr+"-"+capacityStr, func(b *testing.B) {
				benchWriteReadSteady(b, kind, capacity)
			})
		}
	}
}

func benchWriteReadCycle(b *testing.B, kind BufferKind, capacity int) {
	rb := NewRingBuffer[int](uint32(capacity), kind)

	cycles := (b.N + capacity - 1) / capacity
	remainder := b.N % capacity
	if remainder == 0 {
		remainder = capacity
	}

	b.ResetTimer()

	for cycleIdx := range cycles {
		itemsPerCycle := capacity
		if cycleIdx == cycles-1 {
			itemsPerCycle = remainder
		}

		// Fill the buffer
		for val := range itemsPerCycle {
			err := rb.Write(val)
			if err != nil {
				b.Logf("Write error: %v,", err)
				continue
			}
		}

		// Empty the buffer
		for range itemsPerCycle {
			_, err := rb.Read()
			if err != nil {
				b.Logf("Read error: %v", err)
				continue
			}
		}
	}
}

func benchWriteReadSteady(b *testing.B, kind BufferKind, capacity int) {
	rb := NewRingBuffer[int](uint32(capacity), kind)

	val := 0
	for b.Loop() {
		if err := rb.Write(val); err != nil {
			b.Logf("Write error: %v,", err)
			continue
		}

		_, err := rb.Read()
		if err != nil {
			b.Logf("Read error: %v", err)
			continue
		}

		val++
	}
}
