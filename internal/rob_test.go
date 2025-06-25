package internal

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var robCfg = &ROBConfig{
	WindowSize:              32,
	MaxSeqNum:               255,
	TimeAnomalyLowerBound:   100 * time.Millisecond,
	TimeAnomalyUpperBound:   1 * time.Second,
	FallbackInterval:        1 * time.Millisecond,
	MaxSamples:              64,
	KeptSamples:             32,
	SampleEstimateTreshold:  32,
	SampleEstimateFrequency: 8,
}

func initROBTest() *ROB[*dummyReOrderable] {
	tel := NewTelemetry("test", "rob")
	return NewROB[*dummyReOrderable](tel, robCfg)
}

type dummyReOrderable struct {
	seqNum      uint64
	recvTime    time.Time
	logicalTime time.Time
}

func (d *dummyReOrderable) SequenceNumber() uint64 {
	return d.seqNum
}

func (d *dummyReOrderable) ReceiveTime() time.Time {
	return d.recvTime
}

func (d *dummyReOrderable) LogicalTime() time.Time {
	return d.logicalTime
}

func (d *dummyReOrderable) SetLogicalTime(logicalTime time.Time) {
	d.logicalTime = logicalTime
}

func getItemSequential(baseTime time.Time, maxSeqNum, count, from int) []*dummyReOrderable {
	items := make([]*dummyReOrderable, 0, count)
	for i := range count {
		seqNum := uint64((i + from) % (maxSeqNum + 1))
		recvTime := baseTime.Add(time.Duration(i) * time.Millisecond)
		items = append(items, &dummyReOrderable{seqNum: seqNum, recvTime: recvTime})
	}
	return items
}

func Test_ROB_EnqueueSequential(t *testing.T) {
	assert := assert.New(t)

	rob := initROBTest()

	baseTime := time.Now()

	itemCount := 256
	items := getItemSequential(baseTime, 255, itemCount, 0)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		outputCh := rob.GetOutputCh()
		delivered := 0
		expected := uint64(0)
		for item := range outputCh {
			assert.Equal(expected, item.SequenceNumber())

			logicalTime := baseTime.Add(time.Duration(expected) * time.Millisecond)
			assert.Equal(logicalTime, item.LogicalTime())

			delivered++
			if delivered == itemCount {
				break
			}

			expected++
		}
	}()

	for _, item := range items {
		assert.NoError(rob.Enqueue(item))
	}

	wg.Wait()
}

func Test_ROB_EnqueueOutOfOrder(t *testing.T) {
	assert := assert.New(t)

	baseTime := time.Now()

	items := []*dummyReOrderable{}
	chunk := 0
	chunkSize := 16
	chunkSeqNum := make(map[uint64]struct{})

	for i := range 256 {
		if i != 0 && i%chunkSize == 0 {
			chunk++
			chunkSeqNum = make(map[uint64]struct{})
		}

		var seqNum uint64
		for {
			randSeqNum := uint64(rand.Int() % chunkSize)
			if _, ok := chunkSeqNum[randSeqNum]; !ok {
				chunkSeqNum[randSeqNum] = struct{}{}
				seqNum = randSeqNum + uint64(chunk*chunkSize)
				break
			}
		}

		recvTime := baseTime.Add(time.Duration(i) * time.Millisecond)
		items = append(items, &dummyReOrderable{seqNum: seqNum, recvTime: recvTime})
	}

	allSeqNum := make(map[uint64]int)
	for i := range uint64(256) {
		allSeqNum[i] = 0
	}

	for _, item := range items {
		count, ok := allSeqNum[item.seqNum]
		assert.True(ok)
		assert.Equal(0, count)
		allSeqNum[item.seqNum]++
	}
	assert.Len(allSeqNum, 256)

	rob := initROBTest()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		outputCh := rob.GetOutputCh()
		count := uint(0)
		for item := range outputCh {
			logicalTime := baseTime.Add(time.Duration(count) * time.Millisecond)
			assert.Equal(logicalTime, item.LogicalTime())

			tmpSeqNum := item.SequenceNumber()
			assert.Equal(1, allSeqNum[tmpSeqNum])
			allSeqNum[tmpSeqNum]++

			count++
			if count == 256 {
				break
			}
		}
	}()

	for _, item := range items {
		assert.NoError(rob.Enqueue(item))
	}

	wg.Wait()
}

func Test_ROB_EnqueueOffset(t *testing.T) {
	assert := assert.New(t)

	rob := initROBTest()

	baseTime := time.Now()
	itemCount := 256
	offset := 32
	items := getItemSequential(baseTime, 255, itemCount, offset)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		outputCh := rob.GetOutputCh()
		expected := uint64(offset)
		delivered := 0
		for item := range outputCh {
			assert.Equal(expected%256, item.SequenceNumber())

			delivered++
			if delivered == itemCount {
				break
			}

			expected++
		}
	}()

	for _, item := range items {
		assert.NoError(rob.Enqueue(item))
	}

	wg.Wait()
}

func Test_ROB_EnqueueErrors(t *testing.T) {
	assert := assert.New(t)

	rob := initROBTest()

	items := getItemSequential(time.Now(), 255, 128, 0)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		outputCh := rob.GetOutputCh()
		delivered := 0
		expected := uint64(0)
		for item := range outputCh {
			assert.Equal(expected%256, item.SequenceNumber())

			delivered++
			if delivered == 128 {
				break
			}
			expected++
		}
	}()

	for _, item := range items {
		assert.NoError(rob.Enqueue(item))
	}

	// Check max sequence number
	assert.ErrorIs(rob.Enqueue(&dummyReOrderable{seqNum: 256}), ErrMaxSeqNumExceeded)

	// Check out of window sequence numbers
	assert.ErrorIs(rob.Enqueue(&dummyReOrderable{seqNum: 0}), ErrSeqNumOutOfWindow)
	assert.ErrorIs(rob.Enqueue(&dummyReOrderable{seqNum: 127}), ErrSeqNumOutOfWindow)
	assert.ErrorIs(rob.Enqueue(&dummyReOrderable{seqNum: 160}), ErrSeqNumOutOfWindow)

	// Check duplicated sequence number
	assert.NoError(rob.Enqueue(&dummyReOrderable{seqNum: 159}))
	assert.ErrorIs(rob.Enqueue(&dummyReOrderable{seqNum: 159}), ErrSeqNumDuplicated)

	wg.Wait()
}

func Test_ROB_Flush(t *testing.T) {
	assert := assert.New(t)

	rob := initROBTest()

	baseTime := time.Now()

	wg := &sync.WaitGroup{}
	wg.Add(1)

	inSeqCount := 40
	inSeqItems := getItemSequential(baseTime, 255, inSeqCount, 0)

	jumpSeqCount := 16
	jumpSeqFrom := inSeqCount + 1
	jumpSeqItems := getItemSequential(baseTime, 255, jumpSeqCount, jumpSeqFrom)

	go func() {
		defer wg.Done()

		outputCh := rob.GetOutputCh()
		delivered := 0
		expectedSeqNum := uint64(0)
		for item := range outputCh {
			assert.Equal(expectedSeqNum, item.SequenceNumber())
			logicalTime := baseTime.Add(time.Duration(expectedSeqNum) * time.Millisecond)
			assert.Equal(logicalTime, item.LogicalTime())

			delivered++
			if delivered == inSeqCount+jumpSeqCount {
				break
			}

			if delivered == inSeqCount {
				expectedSeqNum = uint64(inSeqCount + 1)
			} else {
				expectedSeqNum++
			}
		}
	}()

	for _, item := range inSeqItems {
		assert.NoError(rob.Enqueue(item))
	}

	t.Logf("Enqueued items in sequence until sequence number %d", inSeqCount-1)

	for _, item := range jumpSeqItems {
		assert.NoError(rob.Enqueue(item))
	}

	t.Logf("Enqueued items with jump from sequence number %d to %d", jumpSeqFrom, jumpSeqFrom+jumpSeqCount-1)

	rob.Flush()

	assert.Len(rob.buf, 32)
	for _, item := range rob.buf {
		assert.Nil(item)
	}
	assert.Equal(uint64(0), rob.bufItems)
	assert.Equal(uint64(0), rob.enqueuedItems)
	assert.Equal(uint64(0), rob.nextSeqNum)

	wg.Wait()
}

func Test_ROB_FlushTimeout(t *testing.T) {
	assert := assert.New(t)

	baseTime := time.Now()

	rob := initROBTest()

	offset := 1
	itemCount := 16
	items := getItemSequential(baseTime, 255, itemCount, offset)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		outputCh := rob.GetOutputCh()
		delivered := 0
		expected := uint64(offset)
		for item := range outputCh {
			assert.Equal(expected%256, item.SequenceNumber())

			delivered++
			if delivered == itemCount {
				break
			}

			expected++
		}
	}()

	timeout := time.NewTimer(time.Second)

	enqueueCh := make(chan *dummyReOrderable)

	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-timeout.C:
				rob.Flush()
				return
			case item := <-enqueueCh:
				assert.NoError(rob.Enqueue(item))
			}
		}
	}()

	for _, item := range items {
		enqueueCh <- item
	}

	wg.Wait()
}
