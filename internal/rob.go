package internal

import (
	"errors"
	"log"
	"time"
)

var (
	ErrMaxSeqNumExceeded = errors.New("maximum sequence number exceeded")
	ErrSeqNumOutOfWindow = errors.New("sequence number out of window")
	ErrSeqNumDuplicated  = errors.New("sequence number duplicated")
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

	enqueuedItems uint64

	nextSeqNum uint
	maxSeqNum  uint

	// timeAnomalyLowerBound is the lower bound for time anomaly detection
	timeAnomalyLowerBound time.Duration
	// timeAnomalyUpperBound is the upper bound for time anomaly detection
	timeAnomalyUpperBound time.Duration

	// baseSampleTime holds the base sample time that is used
	// to calculate the logical time by adding the sample interval
	baseSampleTime time.Time
	// sampleInterval holds the current sample interval.
	// It is basically the estimated time difference between items
	sampleInterval time.Duration
	// fallbackInterval is the fallback value for the sample interval
	fallbackInterval time.Duration

	// samples holds the time differences between items (samples)
	samples []time.Duration
	// sampleCount is the number of samples currently stored
	sampleCount int
	// keptSamples is the number of samples that are kept
	// when maxSamples is reached
	keptSamples int
	// maxSamples is the maximum number of samples that can be stored
	maxSamples int
	// sampleEstimateTreshold is the minimum number of samples
	// that must be present to start estimating the sample interval
	sampleEstimateTreshold int
	// isSampleIntervalStable states whether the sample interval is stable
	isSampleIntervalStable bool

	// lastRecvTime holds the receive time of the last enqueued item.
	// It is used to calculate the time difference between items
	lastRecvTime time.Time
	// lastSeqNum holds the sequence number of the last enqueued item.
	// It is used to calculate the distance between items
	lastSeqNum uint

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

		timeAnomalyLowerBound: time.Millisecond * 500,
		timeAnomalyUpperBound: time.Second * 5,

		baseSampleTime: time.Now(),

		sampleInterval:   fbInterval,
		fallbackInterval: fbInterval,

		samples:                make([]time.Duration, maxSamples),
		sampleCount:            0,
		keptSamples:            maxSamples / 2,
		maxSamples:             maxSamples,
		sampleEstimateTreshold: maxSamples / 2,

		deliverCh: deliverCh,
	}
}

// getSeqNumDistance returns the distance between the current
// and the provided sequence number.
func (rob *ROB) getSeqNumDistance(curr, against uint) uint {
	if curr >= against {
		return curr - against
	}

	// Wraparound case
	return rob.maxSeqNum + curr + 1 - against
}

// verifySequenceNumber checks if the sequence number is valid and returns the buffer index.
func (rob *ROB) verifySequenceNumber(seqNum uint) (uint, error) {
	// Check if the sequence number exceeds the maximum
	if seqNum > rob.maxSeqNum {
		return 0, ErrMaxSeqNumExceeded
	}

	distance := rob.getSeqNumDistance(seqNum, rob.nextSeqNum)

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

	// Check if the sequence number is valid
	bufIdx, err := rob.verifySequenceNumber(seqNum)
	if err != nil {
		return false
	}

	recvTime := item.ReceiveTime()

	// If the item is the first, calibrate the base time
	if rob.enqueuedItems == 0 {
		rob.calibrateBaseTime(seqNum, recvTime)
	}

	// Check if a time anomaly is detected.
	// If so, reset the sample interval and calibrate the base time
	if rob.detectTimeAnomaly(recvTime) {
		log.Print("Time anomaly detected, recalibrating")
		rob.resetSampleInterval()
		rob.calibrateBaseTime(seqNum, recvTime)
	}

	// Sample the item
	rob.sampleItem(seqNum, recvTime)
	rob.enqueuedItems++

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

	rob.deliverCh <- item
}

// calibrateBaseTime sets the base sample time to the provided receive time.
func (rob *ROB) calibrateBaseTime(seqNum uint, recvTime time.Time) {
	rob.baseSampleTime = recvTime

	log.Printf("Calibrated base time: seq=%d, recv=%v, base=%v",
		seqNum, recvTime, rob.baseSampleTime)
}

// detectTimeAnomaly returns true if the given receive time is
// very different from the last receive time.
func (rob *ROB) detectTimeAnomaly(recvTime time.Time) bool {
	if rob.lastRecvTime.IsZero() {
		return false
	}

	// Check if receive time goes backwards by more than lower bound
	if recvTime.Before(rob.lastRecvTime.Add(-rob.timeAnomalyLowerBound)) {
		return true
	}

	// Check if receive time jumps forward by more than upper bound
	if recvTime.After(rob.lastRecvTime.Add(rob.timeAnomalyUpperBound)) {
		return true
	}

	return false
}

// sampleItem adds the current item's time difference to the samples
// if it is consecutive to the last item.
func (rob *ROB) sampleItem(seqNum uint, recvTime time.Time) {
	var sample time.Duration

	// Skip if this is the first item
	if rob.lastRecvTime.IsZero() {
		goto update
	}

	// Skip if the sequence number is not consecutive
	if rob.getSeqNumDistance(seqNum, rob.lastSeqNum) != 1 {
		goto update
	}

	// Calculate the difference between the last and the current receive time
	sample = recvTime.Sub(rob.lastRecvTime)

	// Reject obviously wrong intervals
	if sample > 0 && sample < time.Second {
		rob.addSample(sample)
	}

update:
	rob.lastSeqNum = seqNum
	rob.lastRecvTime = recvTime
}

// addSample adds the provided sample and calculates the interval
// if the number of samples has reached the threshold.
func (rob *ROB) addSample(sample time.Duration) {
	// Check if the maximum number of samples has been reached
	if rob.sampleCount >= rob.maxSamples {
		// Shift samples left to make room for the new sample
		copy(rob.samples, rob.samples[rob.keptSamples:])
		rob.sampleCount = rob.keptSamples
	} else {
		rob.sampleCount++
	}

	rob.samples[rob.sampleCount-1] = sample

	// Calculate sample interval if the number of samples has reached the threshold
	if rob.sampleCount > rob.sampleEstimateTreshold {
		rob.calculateSampleInterval()
	}
}

// calculateSampleInterval calculates the sample interval.
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

	// Calculate average duration distance between samples
	avgDuration := sum / time.Duration(rob.sampleCount)

	// Calculate jitters
	jitter := max - min
	maxJitter := avgDuration / 4

	// Check if samples are not stable by checking jitter
	if jitter > maxJitter && rob.isSampleIntervalStable {
		log.Printf("High jitter detected (%v), reverting to fallback interval %v",
			jitter, rob.fallbackInterval)
		rob.resetSampleInterval()
		return
	}

	// Update sample interval if not stable yet
	if !rob.isSampleIntervalStable {
		// Set the new sample interval to the average
		// duration distance between samples
		rob.sampleInterval = avgDuration

		rob.isSampleIntervalStable = true

		log.Printf("Interval updated: %v (jitter: %v, samples: %d)",
			avgDuration, jitter, rob.sampleCount)
	}
}

// resetSampleInterval resets all the variables related to the sample interval.
func (rob *ROB) resetSampleInterval() {
	rob.sampleInterval = rob.fallbackInterval
	clear(rob.samples)
	rob.sampleCount = 0
	rob.isSampleIntervalStable = false
	rob.lastRecvTime = time.Time{}
}
