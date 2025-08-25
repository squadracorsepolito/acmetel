package rb

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	itemsCount              = 1_000_000
	bufferCapacity          = 128
	benchmarkBufferCapacity = 2048
)

func Test_spscBuffer(t *testing.T) {
	assert := assert.New(t)
	b := newSPSCBuffer[int](bufferCapacity)
	testBuffer(assert, 1, 1, b)
}

func Test_spsc2Buffer(t *testing.T) {
	assert := assert.New(t)
	b := newSPSC2Buffer[int](bufferCapacity)
	testBuffer(assert, 1, 1, b)
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

func Benchmark_RingBuffers(b *testing.B) {
	b.ReportAllocs()

	kinds := []BufferKind{BufferKindSPSC, BufferKindSPSC2}
	for _, kind := range kinds {
		kindStr := kind.String()

		b.Run("WriteSequential-"+kindStr, func(b *testing.B) {
			benchmarkWriteSequential(b, kind)
		})

		b.Run("ReadSequential-"+kindStr, func(b *testing.B) {
			benchmarkReadSequential(b, kind)
		})

		// if kind != BufferKindSPSC {
		// 	b.Run("WriteParallel-"+kindStr, func(b *testing.B) {
		// 		benchmarkWriteParallel(b, kind)
		// 	})

		// 	b.Run("ReadParallel-"+kindStr, func(b *testing.B) {
		// 		benchmarkReadParallel(b, kind)
		// 	})
		// }
	}
}

func newBenchRingBuffer(kind BufferKind) *RingBuffer[int] {
	return NewRingBuffer[int](benchmarkBufferCapacity, kind)
}

func benchmarkWriteSequential(b *testing.B, kind BufferKind) {
	rb := newBenchRingBuffer(kind)

	go func() {
		for {
			_, err := rb.Read()
			if err != nil {
				b.Logf("Read error: %v,", err)
				return
			}
		}
	}()

	for b.Loop() {
		err := rb.Write(1)
		if err != nil {
			b.Logf("Write error: %v,", err)
			return
		}
	}
}

func benchmarkReadSequential(b *testing.B, kind BufferKind) {
	rb := newBenchRingBuffer(kind)

	go func() {
		for range b.N {
			err := rb.Write(1)
			if err != nil {
				b.Logf("Write error: %v,", err)
				return
			}
		}
	}()

	b.ResetTimer()
	for range b.N {
		_, err := rb.Read()
		if err != nil {
			b.Logf("Read error: %v", err)
			return
		}
	}
}

func benchmarkWriteParallel(b *testing.B, kind BufferKind) {
	rb := newBenchRingBuffer(kind)

	go func() {
		for {
			_, err := rb.Read()
			if err != nil {
				b.Logf("Read error: %v,", err)
				return
			}
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			err := rb.Write(1)
			if err != nil {
				b.Logf("Write error: %v,", err)
				return
			}
		}
	})
}

func benchmarkReadParallel(b *testing.B, kind BufferKind) {
	rb := newBenchRingBuffer(kind)

	go func() {
		for range b.N {
			err := rb.Write(1)
			if err != nil {
				b.Logf("Write error: %v,", err)
				return
			}
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := rb.Read()
			if err != nil {
				b.Logf("Read error: %v", err)
				return
			}
		}
	})
}
