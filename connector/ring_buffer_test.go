package connector

import (
	"errors"
	"log"
	"sync"
	"testing"
)

const (
	testSize      = uint64(4096)
	testItemCount = 1_000_000

	benchSize     = 4096
	benchDataSize = 1024
)

func testConnector(t *testing.T, connector Connector[int], itemCount int) {
	readerWg := sync.WaitGroup{}
	readerWg.Add(1)

	expected := make(map[int]int)
	for idx := range itemCount {
		expected[idx] = 0
	}

	counted := 0
	duplicated := 0
	go func() {
		defer readerWg.Done()

		for {
			item, err := connector.Read()
			if err != nil {
				if !errors.Is(err, ErrClosed) {
					t.Logf("Read error: %v", err)
				}
				return
			}

			counted++
			expected[item]++

			if expected[item] > 1 {
				duplicated++
			}
		}
	}()

	for idx := range itemCount {
		err := connector.Write(idx)
		if err != nil {
			t.Logf("Write error: %v,", err)
			continue
		}
	}

	connector.Close()

	readerWg.Wait()

	if counted != itemCount {
		t.Errorf("Expected %d items, got %d", itemCount, counted)
	}

	dupPerc := float64(duplicated) / float64(itemCount)
	t.Logf("Duplicated %d over %d items, percentage of duplicates: %f", duplicated, itemCount, dupPerc)

	percTreshold := 0.001
	if dupPerc > percTreshold {
		t.Errorf("Expected max %f precentage of duplicated items, got %f", percTreshold, dupPerc)
	}
}

// func Test_RingBuffer(t *testing.T) {
// 	testConnector(t, NewRingBuffer[int](testSize), testItemCount)
// }

// func Test_Channel(t *testing.T) {
// 	testConnector(t, NewChannel[int](testSize), testItemCount)
// }

func Benchmark_Connectors(b *testing.B) {
	b.ReportAllocs()

	connKinds := []string{"ring_buffer_2"}
	for _, connKind := range connKinds {
		b.Run("Sequential-"+connKind, func(b *testing.B) {
			benchmarkConnectorSequential(b, connKind, benchSize, benchDataSize)
		})

		b.Run("Sequential2-"+connKind, func(b *testing.B) {
			benchmarkConnectorSequential2(b, connKind, benchSize, benchDataSize)
		})

		// b.Run("Parallel-"+connKind, func(b *testing.B) {
		// 	benchmarkConnectorParallel(b, connKind, benchSize, benchDataSize, 2, 2)
		// })
	}
}

func getConnectorFormKind[T any](connKind string, size uint64) Connector[T] {
	var connector Connector[T]
	switch connKind {
	case "channel":
		connector = NewChannel[T](size)
	case "ring_buffer":
		connector = NewRingBuffer[T](size)
	case "ring_buffer_2":
		connector = NewRingBuffer2[T](uint32(size))
	}
	return connector
}

func benchmarkConnectorSequential(b *testing.B, connKind string, size uint64, dataSize int) {
	connector := getConnectorFormKind[[]byte](connKind, size)

	data := make([]byte, dataSize)

	b.ResetTimer()
	for b.Loop() {
		err := connector.Write(data)
		if err != nil {
			b.Logf("Write error: %v,", err)
			return
		}
		_, err = connector.Read()
		if err != nil {
			b.Logf("Read error: %v", err)
			return
		}
	}
}

func benchmarkConnectorSequential2(b *testing.B, connKind string, size uint64, dataSize int) {
	type dummy struct {
		data []byte
	}

	connector := getConnectorFormKind[*dummy](connKind, size)

	data := make([]byte, dataSize)
	d := &dummy{data}

	b.ResetTimer()
	for b.Loop() {
		err := connector.Write(d)
		if err != nil {
			b.Logf("Write error: %v,", err)
			return
		}
		_, err = connector.Read()
		if err != nil {
			b.Logf("Read error: %v", err)
			return
		}
	}
}

func benchmarkConnectorParallel(b *testing.B, connKind string, size uint64, dataSize, writerCount, readerCount int) {
	type dummy struct {
		data []byte
	}

	connector := getConnectorFormKind[*dummy](connKind, size)

	data := &dummy{
		data: make([]byte, dataSize),
	}

	readerWg := sync.WaitGroup{}
	readerWg.Add(readerCount)

	// var itemReadCount atomic.Uint64

	for range readerCount {
		go func() {
			defer readerWg.Done()

			for range b.N / readerCount {
				_, err := connector.Read()
				if err != nil {
					b.Logf("Read error: %v", err)
					return
				}

				// itemReadCount.Add(1)

				// if itemReadCount.Load() == uint64(b.N) {
				// 	connector.Close()
				// }
			}
		}()
	}

	writerWg := sync.WaitGroup{}
	writerWg.Add(writerCount)

	for range writerCount {
		go func() {
			defer writerWg.Done()

			for range b.N / writerCount {
				err := connector.Write(data)
				if err != nil {
					b.Logf("Write error: %v,", err)
					return
				}
			}
		}()
	}

	b.ResetTimer()

	writerWg.Wait()

	log.Print("WRITER DONE")

	readerWg.Wait()
}
