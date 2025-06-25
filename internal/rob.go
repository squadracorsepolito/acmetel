package internal

import (
	"errors"
	"time"
)

var (
	// ErrMaxSeqNumExceeded is returned when the maximum sequence number is exceeded.
	ErrMaxSeqNumExceeded = errors.New("maximum sequence number exceeded")
	// ErrSeqNumOutOfWindow is returned when the sequence number is out of the window.
	ErrSeqNumOutOfWindow = errors.New("sequence number out of window")
	// ErrSeqNumDuplicated is returned when the sequence number is duplicated.
	ErrSeqNumDuplicated = errors.New("sequence number duplicated")
)

type reOrderable interface {
	SequenceNumber() uint64
	ReceiveTime() time.Time
	LogicalTime() time.Time
	SetLogicalTime(logicalTime time.Time)
}

// ROBConfig is the configuration for the re-ordering buffer structure [ROB].
type ROBConfig struct {
	// WindowSize is the size of the re-ordering buffer window
	WindowSize uint64
	// MaxSeqNum is the maximum value of the sequence number
	MaxSeqNum uint64

	// TimeAnomalyLowerBound is the lower bound for time anomaly detection
	TimeAnomalyLowerBound time.Duration
	// TimeAnomalyUpperBound is the upper bound for time anomaly detection
	TimeAnomalyUpperBound time.Duration

	// FallbackInterval is the fallback interval to use when the sample interval is not stable
	FallbackInterval time.Duration

	// MaxSamples is the maximum number of samples to keep
	MaxSamples int
	// KeptSamples is the number of samples to keep when the
	// maximum number of samples is reached
	KeptSamples int
	// SampleEstimateTreshold is the minimum number of samples
	// required to estimate the sample interval
	SampleEstimateTreshold int
	// SampleEstimateFrequency is the number of new samples
	// required to start a new estimation
	SampleEstimateFrequency int
}

// ROB structure implements a re-ordering buffer.
type ROB[T reOrderable] struct {
	tel *Telemetry

	// outputCh is the channel that is used to send items after they are re-ordered
	outputCh chan T

	// windowSize is the size of the re-ordering buffer window
	windowSize uint64
	// maxSeqNum is the maximum value of the sequence number
	maxSeqNum uint64
	// nextSeqNum holds the value of the sequence number that is expected
	// to be enqueued next
	nextSeqNum uint64

	// buf holds the items in the re-ordering buffer
	buf []T
	// bufItems is the number of items currently stored in the buffer
	bufItems uint64
	// bufBitmap is the bitmap that keeps track
	// of the sequence numbers in the buffer
	bufBitmap *bitmap

	// enqueuedItems is the number of items that have been enqueued.
	// It is reset when the buffer is flushed
	enqueuedItems uint64

	// timeAnomalyLowerBound is the lower bound for time anomaly detection
	timeAnomalyLowerBound time.Duration
	// timeAnomalyUpperBound is the upper bound for time anomaly detection
	timeAnomalyUpperBound time.Duration

	// baseTime holds the time that is used to calculate
	// the logical time by adding the interval estimated
	baseTime time.Time
	// interval is the estimed time difference between enqueued items
	interval time.Duration
	// isIntervalStable states whether the estimed interval is stable
	isIntervalStable bool
	// fallbackInterval is the fallback value for the interval
	fallbackInterval time.Duration

	// samples holds the time differences between items (samples)
	samples []time.Duration
	// sampleCount is the number of samples currently stored
	sampleCount int
	// maxSamples is the maximum number of samples that can be stored
	maxSamples int
	// keptSamples is the number of samples that are kept
	// when maxSamples is reached
	keptSamples int
	// sampleEstimateTreshold is the minimum number of samples
	// that must be present to start estimating the sample interval
	sampleEstimateTreshold int
	// sampleEstimateFrequency is the number of samples that must be present
	// above the sampleEstimateTreshold before the sample interval is estimated
	sampleEstimateFrequency int

	// lastRecvTime holds the receive time of the last enqueued item.
	// It is used to calculate the time difference between items
	lastRecvTime time.Time
	// lastSeqNum holds the sequence number of the last enqueued item.
	// It is used to calculate the distance between items
	lastSeqNum uint64
}

// NewROB returns a new [ROB] (re-ordering buffer).
func NewROB[T reOrderable](tel *Telemetry, cfg *ROBConfig) *ROB[T] {
	return &ROB[T]{
		tel: tel,

		outputCh: make(chan T, cfg.WindowSize),

		windowSize: cfg.WindowSize,
		maxSeqNum:  cfg.MaxSeqNum,
		nextSeqNum: 0,

		buf:       make([]T, cfg.WindowSize),
		bufItems:  0,
		bufBitmap: newBitmap(cfg.WindowSize),

		enqueuedItems: 0,

		timeAnomalyLowerBound: cfg.TimeAnomalyLowerBound,
		timeAnomalyUpperBound: cfg.TimeAnomalyUpperBound,

		baseTime:         time.Now(),
		interval:         cfg.FallbackInterval,
		isIntervalStable: false,
		fallbackInterval: cfg.FallbackInterval,

		samples:                 make([]time.Duration, cfg.MaxSamples),
		sampleCount:             0,
		maxSamples:              cfg.MaxSamples,
		keptSamples:             cfg.KeptSamples,
		sampleEstimateTreshold:  cfg.SampleEstimateTreshold,
		sampleEstimateFrequency: cfg.SampleEstimateFrequency,

		lastRecvTime: time.Time{},
		lastSeqNum:   0,
	}
}

// GetOutputCh returns the output channel of the re-ordering buffer.
// It is used to receive items after they are re-ordered.
func (rob *ROB[T]) GetOutputCh() chan T {
	return rob.outputCh
}

// Enqueue adds the provided item to the re-ordering buffer.
// If the sequence number of the item matches the next sequence number,
// the item is delivered immediately to the output channel.
// Otherwise, the item is added to the re-ordering buffer.
// The buffer is flushed automatically when it contains a complete sequence of items.
// If the buffer sequence is not complete, it must be flushed manually.
//
// It return an error if the item's sequence number is invalid.
//
// ATTENTION: The given item cannot be nil.
func (rob *ROB[T]) Enqueue(item T) error {
	seqNum := item.SequenceNumber()
	recvTime := item.ReceiveTime()

	// Check if the item is the first of the sequence or just after a flush
	if rob.enqueuedItems%(rob.maxSeqNum+1) == 0 {
		// If the sequence number is greater than the window size,
		// take it as the next sequence number
		if seqNum >= rob.windowSize {
			rob.nextSeqNum = seqNum
		}

		rob.calibrateBaseTime(seqNum, recvTime)
	}

	// Check if the sequence number is valid
	bufIdx, err := rob.verifySequenceNumber(seqNum)
	if err != nil {
		return err
	}

	// Check if a time anomaly is detected.
	// If so, reset the sample interval and calibrate the base time
	if rob.detectTimeAnomaly(recvTime) {
		rob.resetInterval()
		rob.calibrateBaseTime(seqNum, recvTime)
	}

	// Sample the item
	rob.sampleItem(seqNum, recvTime)
	rob.enqueuedItems++

	// Check if the item is in sequence
	if seqNum == rob.nextSeqNum {
		rob.deliver(item)

		if rob.bufItems == 0 {
			return nil
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
			rob.buf[rob.windowSize-i-1] = *new(T)
		}

		// Adjust the buffer items count
		rob.bufItems -= itemsToDeliver - 1

		return nil
	}

	// The item has to be placed in the buffer
	rob.buf[bufIdx] = item
	rob.bufItems++

	// Set the corresponding bit in the bitmap
	rob.bufBitmap.set(bufIdx)

	return nil
}

// Flush flushes the re-ordering buffer,
// so it delivers all the items to the output channel.
// Once called, the buffer is reset.
// This function is meant to be called when the caller notices
// that the buffer is not returning items because of a missing item
// in the sequence, i.e. a lost packet, or when the caller is exiting.
// Because of the former case, a timeout policy should be implemented,
// but be careful to the single-threaded nature of the re-ordering buffer
// (you cannot enqueue items while flushing).
func (rob *ROB[T]) Flush() {
	for idx, item := range rob.buf {
		if !rob.bufBitmap.isSet(uint64(idx)) {
			continue
		}

		rob.setLogicalTime(item)
		rob.outputCh <- item
	}

	rob.resetBuf()
	rob.resetInterval()
}

// getSeqNumDistance returns the distance between the current
// and the provided sequence number.
func (rob *ROB[T]) getSeqNumDistance(curr, against uint64) uint64 {
	if curr >= against {
		return curr - against
	}

	// Wraparound case
	return rob.maxSeqNum + curr + 1 - against
}

// verifySequenceNumber checks if the sequence number is valid and returns the buffer index.
func (rob *ROB[T]) verifySequenceNumber(seqNum uint64) (uint64, error) {
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

// resetBuf resets all the variables related to the buffer.
func (rob *ROB[T]) resetBuf() {
	clear(rob.buf)
	rob.bufItems = 0
	rob.enqueuedItems = 0
	rob.bufBitmap.reset()
	rob.nextSeqNum = 0
}

// setLogicalTime sets the logical time of the provided item.
func (rob *ROB[T]) setLogicalTime(item T) {
	seqNum := item.SequenceNumber()
	logicalTime := rob.baseTime.Add(time.Duration(seqNum) * rob.interval)
	item.SetLogicalTime(logicalTime)
}

// deliver delivers the provided item and increments the next sequence number.
func (rob *ROB[T]) deliver(item T) {
	rob.setLogicalTime(item)

	rob.nextSeqNum++
	if rob.nextSeqNum > rob.maxSeqNum {
		rob.nextSeqNum = 0
	}

	rob.outputCh <- item
}

// calibrateBaseTime sets the base sample time to the provided receive time.
func (rob *ROB[T]) calibrateBaseTime(seqNum uint64, recvTime time.Time) {
	rob.baseTime = recvTime

	rob.tel.LogInfo("calibrate base time",
		"sequence_number", seqNum,
		"receive_time", recvTime,
	)
}

// detectTimeAnomaly returns true if the given receive time is
// very different from the last receive time.
func (rob *ROB[T]) detectTimeAnomaly(recvTime time.Time) bool {
	if rob.lastRecvTime.IsZero() {
		return false
	}

	// Check if receive time goes backwards by more than lower bound
	if recvTime.Before(rob.lastRecvTime.Add(-rob.timeAnomalyLowerBound)) {
		rob.tel.LogWarn("time anomaly detected",
			"direction", "backward",
			"receive_time", recvTime,
			"last_recv_time", rob.lastRecvTime,
			"lower_bound", rob.timeAnomalyLowerBound,
		)

		return true
	}

	// Check if receive time jumps forward by more than upper bound
	if recvTime.After(rob.lastRecvTime.Add(rob.timeAnomalyUpperBound)) {
		rob.tel.LogWarn("time anomaly detected",
			"direction", "forward",
			"receive_time", recvTime,
			"last_recv_time", rob.lastRecvTime,
			"upper_bound", rob.timeAnomalyUpperBound,
		)

		return true
	}

	return false
}

// sampleItem adds the current item's time difference to the samples
// if it is consecutive to the last item.
func (rob *ROB[T]) sampleItem(seqNum uint64, recvTime time.Time) {
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
func (rob *ROB[T]) addSample(sample time.Duration) {
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
	// and it is a multiple of the sample estimate frequency
	if rob.sampleCount >= rob.sampleEstimateTreshold &&
		rob.sampleCount%rob.sampleEstimateFrequency == 0 {
		rob.estimateInterval()
	}
}

// estimateInterval calculates the interval based on the samples.
func (rob *ROB[T]) estimateInterval() {
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
	if jitter > maxJitter && rob.isIntervalStable {
		rob.tel.LogInfo("high jitter detected", "jitter", jitter)
		// rob.resetInterval()

		rob.isIntervalStable = false

		return
	}

	// Update sample interval if not stable yet
	if !rob.isIntervalStable {
		// Set the new sample interval to the average
		// duration distance between samples
		rob.interval = avgDuration
		rob.isIntervalStable = true

		rob.tel.LogInfo("interval updated", "interval", rob.interval)
	}
}

// resetInterval resets all the variables related to the interval estimation.
func (rob *ROB[T]) resetInterval() {
	if rob.interval != rob.fallbackInterval {
		rob.tel.LogInfo("reset interval", "fallback_interval", rob.fallbackInterval)
		rob.interval = rob.fallbackInterval
	}

	clear(rob.samples)
	rob.sampleCount = 0
	rob.isIntervalStable = false
	rob.lastRecvTime = time.Time{}
}
