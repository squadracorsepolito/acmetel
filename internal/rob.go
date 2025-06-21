package internal

import (
	"errors"
	"log"
	"time"
)

type reOrderable interface {
	SequenceNumber() uint
	ReceiveTime() time.Time
	LogicalTime() time.Time
	SetLogicalTime(logicalTime time.Time)
}

type ROB struct {
	windowSize uint

	buf       []reOrderable
	bufItems  uint
	bufBitmap *bitmap

	deliveredItems uint64

	nextSeqNum uint
	maxSeqNum  uint

	// lastReceiveTime is used to detect time anomalies
	lastReceiveTime time.Time

	baseSampleTime   time.Time
	fallbackInterval time.Duration
	sampleInterval   time.Duration

	samples                []time.Duration
	sampleCount            int
	maxSamples             int
	isSampleIntervalStable bool
	lastSeqNum             uint
	lastSeqTime            time.Time

	lastDeliveredSeqNum uint

	deliverCh chan<- reOrderable
}

func NewROB(windowSize, maxSeqNum uint, deliverCh chan<- reOrderable) *ROB {

	fbInterval := time.Millisecond
	maxSamples := int(windowSize * 2)

	return &ROB{
		windowSize: windowSize,

		buf:       make([]reOrderable, windowSize),
		bufItems:  0,
		bufBitmap: newBitmap(windowSize),

		nextSeqNum: 0,
		maxSeqNum:  maxSeqNum,

		baseSampleTime: time.Now(),

		fallbackInterval: fbInterval,
		sampleInterval:   fbInterval,

		samples:    make([]time.Duration, maxSamples),
		maxSamples: maxSamples,

		deliverCh: deliverCh,
	}
}

var (
	ErrMaxSeqNumExceeded = errors.New("maximum sequence number exceeded")
	ErrSeqNumOutOfWindow = errors.New("sequence number out of window")
	ErrSeqNumDuplicated  = errors.New("sequence number duplicated")
)

// verifySequenceNumber checks if the sequence number is valid and returns the buffer index.
func (rob *ROB) verifySequenceNumber(seqNum uint) (uint, error) {
	// Check if the sequence number exceeds the maximum
	if seqNum > rob.maxSeqNum {
		return 0, ErrMaxSeqNumExceeded
	}

	// Calculate the distance between the sequence number and the next
	var distance uint
	if seqNum >= rob.nextSeqNum {
		distance = seqNum - rob.nextSeqNum
	} else {
		// The sequence number wraps around the max
		distance = rob.maxSeqNum - rob.nextSeqNum + seqNum + 1
	}

	// Check if the distance is within the window
	if distance >= rob.windowSize {
		return 0, ErrSeqNumOutOfWindow
	}

	// Calculate the buffer index
	bufIdx := distance % rob.windowSize

	// Check if the sequence number is duplicated
	if rob.bufBitmap.isSet(bufIdx) {
		return 0, ErrSeqNumDuplicated
	}

	return bufIdx, nil
}

func (rob *ROB) Enqueue(item reOrderable) bool {
	seqNum := item.SequenceNumber()

	bufIdx, err := rob.verifySequenceNumber(seqNum)
	if err != nil {
		return false
	}

	recvTime := item.ReceiveTime()

	rob.updateIntervalEstimate(seqNum, recvTime)

	if seqNum == 0 && rob.bufItems == 0 {
		rob.calibrateBaseTime(seqNum, recvTime)
	}

	if rob.deliveredItems == 0 {
		rob.calibrateBaseTime(seqNum, recvTime)
	}

	if rob.detectTimeAnomaly(recvTime) {
		log.Print("Time anomaly detected, recalibrating")
		rob.resetIntervalEstimate()
		rob.calibrateBaseTime(seqNum, recvTime)
	}

	rob.lastReceiveTime = recvTime

	// log.Printf("tring to add item %d, bufIdx %d, nextSeqNum %d", seqNum, bufIdx, rob.nextSeqNum)

	// Check if the item is in sequence
	if seqNum == rob.nextSeqNum {
		rob.deliver(item)

		if rob.bufItems == 0 {
			return true
		}

		// The buffer must be shifted because it is not empty.
		// Before shifting, set the current sequence number
		// corresponding bit in the bitmap
		rob.bufBitmap.set(bufIdx)

		// Get the items that need to be delivered
		// because they are consecutively set in the bitmap.
		// itemsToDeliver will always be >= 1
		itemsToDeliver := rob.bufBitmap.consume()

		// Deliver the items
		for i := range itemsToDeliver {
			if i == 0 {
				// Skip what would have been the added item
				continue
			}

			rob.deliver(rob.buf[i])
		}

		// Shift the buffer by cutting it, and then set to nil
		// the new buffer slots created on the right side
		// in order to reduce GC pressure
		copy(rob.buf, rob.buf[itemsToDeliver:])
		for i := range itemsToDeliver {
			rob.buf[rob.windowSize-i-1] = nil
		}

		// Adjust the buffer items count
		rob.bufItems -= itemsToDeliver - 1

		return true
	}

	// The item has to be placed in the buffer
	rob.buf[bufIdx] = item
	rob.bufItems++

	// Set the corresponding bit in the bitmap
	rob.bufBitmap.set(bufIdx)

	return true
}

func (rob *ROB) deliver(item reOrderable) {
	seqNum := item.SequenceNumber()

	logicalTime := rob.baseSampleTime.Add(time.Duration(seqNum) * rob.sampleInterval)

	item.SetLogicalTime(logicalTime)

	rob.nextSeqNum++
	if rob.nextSeqNum > rob.maxSeqNum {
		rob.nextSeqNum = 0
	}
	rob.lastDeliveredSeqNum = seqNum

	rob.deliverCh <- item

	rob.deliveredItems++
}

func (rob *ROB) calibrateBaseTime(seqNum uint, recvTime time.Time) {
	rob.baseSampleTime = recvTime

	log.Printf("Calibrated base time: seq=%d, recv=%v, base=%v",
		seqNum, recvTime, rob.baseSampleTime)
}

const maxJitter = 100 * time.Millisecond
const maxJump = 5 * time.Second

func (rob *ROB) detectTimeAnomaly(recvTime time.Time) bool {
	if rob.lastReceiveTime.IsZero() {
		return false
	}

	// Check if receive time goes backwards by more than reasonable network jitter
	if recvTime.Before(rob.lastReceiveTime.Add(-maxJitter)) {
		return true
	}

	// Check if receive time jumps forward by more than expected
	if recvTime.After(rob.lastReceiveTime.Add(maxJump)) {
		return true
	}

	return false
}

func (rob *ROB) updateIntervalEstimate(seqNum uint, recvTime time.Time) {
	// Skip if this is the first packet or if sequence numbers are not consecutive
	if rob.lastSeqTime.IsZero() {
		rob.lastSeqNum = seqNum
		rob.lastSeqTime = recvTime
		return
	}

	// // Calculate expected sequence difference (handling wraparound)
	// var seqDiff uint
	// if seqNum >= rob.lastSeqNum {
	// 	seqDiff = seqNum - rob.lastSeqNum
	// } else {
	// 	// Wraparound case
	// 	seqDiff = (rob.maxSeqNum + 1 - rob.lastSeqNum) + seqNum
	// }

	// if seqDiff == 1 {
	timeDiff := recvTime.Sub(rob.lastSeqTime)

	if timeDiff > 0 && timeDiff < time.Second {
		rob.addIntervalSample(timeDiff)
	}
	// }

	rob.lastSeqNum = seqNum
	rob.lastSeqTime = recvTime
}

func (rob *ROB) addIntervalSample(sample time.Duration) {
	if rob.sampleCount >= rob.maxSamples {
		halfSamples := rob.maxSamples / 2

		// Shift samples to make room for the current
		copy(rob.samples, rob.samples[halfSamples:])
		rob.sampleCount = halfSamples
	} else {
		rob.sampleCount++
	}

	rob.samples[rob.sampleCount-1] = sample

	// Need at least 5 samples to start estimating
	if rob.sampleCount <= rob.maxSamples/2 {
		return
	}

	// Calculate new sample interval
	rob.calculateSampleInterval()
}

func (rob *ROB) calculateSampleInterval() {
	// Calculate sum, min and max
	min := rob.samples[0]
	max := min
	var sum time.Duration

	for i := range rob.sampleCount {
		sample := rob.samples[i]

		sum += sample
		if sample < min {
			min = sample
		}
		if sample > max {
			max = sample
		}
	}

	// Calculate average
	avg := sum / time.Duration(rob.sampleCount)

	// Calculate jitters
	jitter := max - min
	maxJitter := avg / 4

	// Check if samples are not stable by checking jitter
	if jitter > maxJitter && rob.isSampleIntervalStable {
		log.Printf("High jitter detected (%v), reverting to fallback interval %v",
			jitter, rob.fallbackInterval)
		rob.resetIntervalEstimate()
		return
	}

	// Update sample interval if not stable
	if !rob.isSampleIntervalStable {
		oldInterval := rob.sampleInterval
		rob.sampleInterval = avg
		rob.isSampleIntervalStable = true

		log.Printf("Interval updated: %v -> %v (jitter: %v, samples: %d)",
			oldInterval, avg, jitter, rob.sampleCount)
	}
}

func (rob *ROB) resetIntervalEstimate() {
	rob.sampleInterval = rob.fallbackInterval
	clear(rob.samples)
	rob.sampleCount = 0
	rob.isSampleIntervalStable = false
	rob.lastSeqTime = time.Time{}
}

//
//
//

type bitmap struct {
	bits []byte
	num  uint
}

func newBitmap(num uint) *bitmap {
	return &bitmap{
		bits: make([]byte, (num/8)+1),
		num:  num,
	}
}

func (b *bitmap) set(index uint) {
	b.bits[index/8] |= 1 << (7 - index%8)
}

func (b *bitmap) isSet(index uint) bool {
	return b.bits[index/8]&(1<<(7-index%8)) != 0
}

func (b *bitmap) consume() (consumed uint) {
	for idx := range b.num {
		if !b.isSet(idx) {
			break
		}
		consumed++
	}

	if consumed == 0 {
		return
	}

	bitsLen := uint(len(b.bits))
	skipRows := consumed / 8
	shiftVal := uint8(consumed % 8)
	msbMask := uint8(0)
	for i := range shiftVal {
		msbMask |= 1 << (7 - i)
	}

	isFirst := true
	for byteIdx := skipRows; byteIdx < bitsLen; byteIdx++ {
		currByte := b.bits[byteIdx]

		if isFirst {
			isFirst = false
		} else if shiftVal > 0 {
			msb := currByte & msbMask
			lsb := msb >> (8 - shiftVal)
			b.bits[byteIdx-skipRows-1] |= lsb
		}

		b.bits[byteIdx-skipRows] = currByte << shiftVal
	}

	if skipRows > 0 {
		for byteIdx := bitsLen - skipRows; byteIdx < bitsLen; byteIdx++ {
			b.bits[byteIdx] = 0
		}
	}

	return
}

func (b bitmap) reset() {
	for byteIdx := range b.bits {
		b.bits[byteIdx] = 0
	}
}

func (b *bitmap) print() {
	for byteIdx := range b.bits {
		log.Printf("%08b ", b.bits[byteIdx])
	}
}
