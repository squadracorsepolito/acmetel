package internal

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type dummyReOrderable struct {
	seqNum      uint
	recvTime    time.Time
	logicalTime time.Time
}

func newDummyReOrderable(seqNum uint) *dummyReOrderable {
	return &dummyReOrderable{
		seqNum:   seqNum,
		recvTime: time.Now(),
	}
}

func (d *dummyReOrderable) SequenceNumber() uint {
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

func Test_ROB(t *testing.T) {
	assert := assert.New(t)

	deliverCh := make(chan reOrderable)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()

		target := uint(0)
		for item := range deliverCh {
			assert.Equal(target, item.SequenceNumber())

			target++
			if target == 16 {
				target = 0
			}
		}
	}()

	rob := NewROB(4, 15, deliverCh)

	sequence := []uint{0, 3, 2, 1, 7, 6, 5, 4}
	for _, seqNum := range sequence {
		assert.True(rob.Enqueue(newDummyReOrderable(seqNum)))
	}
	assert.False(rob.Enqueue(newDummyReOrderable(16)))

	sequence = []uint{8, 9, 10, 11, 12, 13, 14, 15, 0}
	for _, seqNum := range sequence {
		assert.True(rob.Enqueue(newDummyReOrderable(seqNum)))
	}

	sequence = []uint{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 0, 15, 13, 14, 3, 4, 2, 1, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}
	for _, seqNum := range sequence {
		assert.True(rob.Enqueue(newDummyReOrderable(seqNum)))
	}
	assert.False(rob.Enqueue(newDummyReOrderable(3)))
	assert.True(rob.Enqueue(newDummyReOrderable(2)))
	assert.True(rob.Enqueue(newDummyReOrderable(1)))
	assert.True(rob.Enqueue(newDummyReOrderable(0)))
	assert.True(rob.Enqueue(newDummyReOrderable(15)))

	assert.False(rob.Enqueue(newDummyReOrderable(7)))

	time.Sleep(500 * time.Millisecond)

	close(deliverCh)

	wg.Wait()
}

func Test_ROB_sequential(t *testing.T) {
	assert := assert.New(t)

	deliverCh := make(chan reOrderable)
	baseTime := time.Now()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		count := uint(0)

		for item := range deliverCh {
			assert.Equal(count, item.SequenceNumber())

			logicalTime := baseTime.Add(time.Duration(count) * time.Millisecond)
			assert.Equal(logicalTime, item.LogicalTime())

			count++
			if count == 256 {
				break
			}
		}
	}()

	rob := NewROB(32, 255, deliverCh)
	for i := range 256 {
		recvTime := baseTime.Add(time.Duration(i) * time.Millisecond)
		assert.True(rob.Enqueue(
			&dummyReOrderable{seqNum: uint(i), recvTime: recvTime},
		))
	}

	wg.Wait()
}

func Test_ROB_outOfOrder(t *testing.T) {
	assert := assert.New(t)

	baseTime := time.Now()

	items := []*dummyReOrderable{}
	chunk := 0
	chunkSize := 16
	chunkSeqNum := make(map[uint]struct{})

	for i := range 256 {
		if i != 0 && i%chunkSize == 0 {
			chunk++
			chunkSeqNum = make(map[uint]struct{})
		}

		var seqNum uint
		for {
			randSeqNum := uint(rand.Int() % chunkSize)
			if _, ok := chunkSeqNum[randSeqNum]; !ok {
				chunkSeqNum[randSeqNum] = struct{}{}
				seqNum = randSeqNum + uint(chunk*chunkSize)
				break
			}
		}

		recvTime := baseTime.Add(time.Duration(i) * time.Millisecond)
		items = append(items, &dummyReOrderable{seqNum: seqNum, recvTime: recvTime})
	}

	allSeqNum := make(map[uint]int)
	for i := range uint(256) {
		allSeqNum[i] = 0
	}

	for _, item := range items {
		count, ok := allSeqNum[item.seqNum]
		assert.True(ok)
		assert.Equal(0, count)
		allSeqNum[item.seqNum]++
	}
	assert.Len(allSeqNum, 256)

	deliverCh := make(chan reOrderable)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		count := uint(0)

		for item := range deliverCh {
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

	rob := NewROB(32, 255, deliverCh)
	for _, item := range items {
		assert.True(rob.Enqueue(item))
	}

	wg.Wait()
}
