package connector

import (
	"errors"
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

func Test_RingBuffer(t *testing.T) {
	testConnector(t, NewRingBuffer[int](testSize), testItemCount)
}

func Test_Channel(t *testing.T) {
	testConnector(t, NewChannel[int](testSize), testItemCount)
}

func Benchmark_Connectors(b *testing.B) {
	b.ReportAllocs()

	connKinds := []string{"channel", "ring_buffer"}
	for _, connKind := range connKinds {
		b.Run("Sequential-"+connKind, func(b *testing.B) {
			benchmarkConnectorSequential(b, connKind, benchSize, benchDataSize)
		})
	}
}

func getConnectorFormKind(connKind string, size uint64) Connector[[]byte] {
	var connector Connector[[]byte]
	switch connKind {
	case "channel":
		connector = NewChannel[[]byte](size)
	case "ring_buffer":
		connector = NewRingBuffer[[]byte](size)
	}
	return connector
}

func benchmarkConnectorSequential(b *testing.B, connKind string, size uint64, dataSize int) {
	connector := getConnectorFormKind(connKind, size)
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
