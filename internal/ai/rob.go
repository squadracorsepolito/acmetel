package ai

import (
	"log"
	"time"
)

type SensorReading interface {
	SequenceNumber() uint
	ReceiveTime() time.Time
	Value() float64
}

type TimestampedReading struct {
	SeqNum      uint
	Value       float64
	SampleTime  time.Time // Calculated from sequence
	ReceiveTime time.Time // Original network arrival
}

type SensorROB struct {
	windowSize int

	buf       []SensorReading
	bufItems  int
	bufBitmap *bitmap

	nextSeqNum uint
	maxSeqNum  uint

	// Timing parameters for sensor data
	baseSampleTime   time.Time     // When sequence 0 was sampled
	sampleInterval   time.Duration // Current estimated interval
	fallbackInterval time.Duration // Fallback when inference fails

	// Interval inference
	intervalSamples []time.Duration // Recent interval measurements
	maxSamples      int             // Max samples to keep for averaging
	lastSeqNum      uint
	lastSeqTime     time.Time
	intervalStable  bool // Whether current interval is reliable

	// For detecting gaps/resets
	lastDeliveredSeq uint
	lastReceiveTime  time.Time

	deliverCh chan<- *TimestampedReading
}

func NewSensorROB(windowSize int, seqNumBits uint, fallbackInterval time.Duration, deliverCh chan<- *TimestampedReading) *SensorROB {
	maxSeqNum := uint(1)
	for i := uint(1); i < seqNumBits; i++ {
		maxSeqNum |= 1 << i
	}

	return &SensorROB{
		windowSize: windowSize,

		buf:       make([]SensorReading, windowSize),
		bufItems:  0,
		bufBitmap: newBitmap(windowSize),

		nextSeqNum: 0,
		maxSeqNum:  maxSeqNum,

		fallbackInterval: fallbackInterval,
		sampleInterval:   fallbackInterval, // Start with fallback
		intervalSamples:  make([]time.Duration, 0, 20),
		maxSamples:       20,
		intervalStable:   false,

		baseSampleTime: time.Now(), // Will be calibrated on first packet

		deliverCh: deliverCh,
	}
}

func (rob *SensorROB) Enqueue(item SensorReading) bool {
	seqNum := item.SequenceNumber()
	recvTime := item.ReceiveTime()

	// Update interval estimation
	rob.updateIntervalEstimate(seqNum, recvTime)

	// Calibrate base time on first packet
	if rob.nextSeqNum == 0 && rob.bufItems == 0 {
		rob.calibrateBaseTime(seqNum, recvTime)
	}

	// Basic validation
	if seqNum > rob.maxSeqNum {
		return false
	}

	// Check for major time jumps (sensor reset, clock issues)
	if rob.detectTimeAnomaly(recvTime) {
		log.Printf("Time anomaly detected, recalibrating")
		rob.resetIntervalEstimate()
		rob.calibrateBaseTime(seqNum, recvTime)
	}

	// Calculate wraparound-aware distance
	var distance uint
	if seqNum >= rob.nextSeqNum {
		distance = seqNum - rob.nextSeqNum
	} else {
		distance = (rob.maxSeqNum + 1 - rob.nextSeqNum) + seqNum
	}

	// Check if within window
	if distance >= uint(rob.windowSize) {
		return false
	}

	// Calculate buffer index
	bufIdx := int(distance) % rob.windowSize

	// Check for duplicates
	if rob.bufBitmap.isSet(bufIdx) {
		return false
	}

	rob.lastReceiveTime = recvTime

	// Handle in-sequence delivery
	if seqNum == rob.nextSeqNum {
		rob.deliverWithLogicalTime(item)

		if rob.bufItems == 0 {
			return true
		}

		// Deliver consecutive buffered items
		rob.bufBitmap.set(bufIdx)
		itemsToDeliver := rob.bufBitmap.consume()

		for i := 1; i < itemsToDeliver; i++ {
			rob.deliverWithLogicalTime(rob.buf[i])
		}

		// Shift buffer
		copy(rob.buf, rob.buf[itemsToDeliver:])
		for i := 0; i < itemsToDeliver; i++ {
			rob.buf[rob.windowSize-i-1] = nil
		}

		rob.bufItems -= itemsToDeliver - 1
		return true
	}

	// Buffer the item
	rob.buf[bufIdx] = item
	rob.bufItems++
	rob.bufBitmap.set(bufIdx)

	return true
}

func (rob *SensorROB) deliverWithLogicalTime(item SensorReading) {
	seqNum := item.SequenceNumber()

	// Calculate logical sample time based on sequence number
	var sampleTime time.Time
	if seqNum >= rob.lastDeliveredSeq {
		// Normal case
		seqDiff := seqNum - rob.lastDeliveredSeq
		sampleTime = rob.baseSampleTime.Add(time.Duration(seqNum) * rob.sampleInterval)
	} else {
		// Wraparound case
		seqDiff := (rob.maxSeqNum + 1 - rob.lastDeliveredSeq) + seqNum
		sampleTime = rob.baseSampleTime.Add(time.Duration(seqNum) * rob.sampleInterval)
	}

	timestamped := &TimestampedReading{
		SeqNum:      seqNum,
		Value:       item.Value(),
		SampleTime:  sampleTime,
		ReceiveTime: item.ReceiveTime(),
	}

	rob.nextSeqNum = (seqNum + 1) % (rob.maxSeqNum + 1)
	rob.lastDeliveredSeq = seqNum

	rob.deliverCh <- timestamped
}

func (rob *SensorROB) updateIntervalEstimate(seqNum uint, recvTime time.Time) {
	// Skip if this is the first packet or if sequence numbers are not consecutive
	if rob.lastSeqTime.IsZero() {
		rob.lastSeqNum = seqNum
		rob.lastSeqTime = recvTime
		return
	}

	// Calculate expected sequence difference (handling wraparound)
	var seqDiff uint
	if seqNum >= rob.lastSeqNum {
		seqDiff = seqNum - rob.lastSeqNum
	} else {
		// Wraparound case
		seqDiff = (rob.maxSeqNum + 1 - rob.lastSeqNum) + seqNum
	}

	// Only use consecutive packets for interval estimation
	if seqDiff == 1 {
		timeDiff := recvTime.Sub(rob.lastSeqTime)

		// Sanity check: reject obviously wrong intervals
		if timeDiff > 0 && timeDiff < time.Second {
			rob.addIntervalSample(timeDiff)
		}
	} else if seqDiff > 1 && seqDiff <= 10 {
		// Handle small gaps - estimate interval from multi-packet gap
		timeDiff := recvTime.Sub(rob.lastSeqTime)
		if timeDiff > 0 {
			avgInterval := timeDiff / time.Duration(seqDiff)
			if avgInterval < time.Second {
				rob.addIntervalSample(avgInterval)
			}
		}
	}

	rob.lastSeqNum = seqNum
	rob.lastSeqTime = recvTime
}

func (rob *SensorROB) addIntervalSample(interval time.Duration) {
	// Add new sample
	rob.intervalSamples = append(rob.intervalSamples, interval)

	// Keep only recent samples
	if len(rob.intervalSamples) > rob.maxSamples {
		rob.intervalSamples = rob.intervalSamples[1:]
	}

	// Need at least 5 samples to start estimating
	if len(rob.intervalSamples) < 5 {
		return
	}

	// Calculate statistics
	var sum, min, max time.Duration
	min = rob.intervalSamples[0]
	max = rob.intervalSamples[0]

	for _, sample := range rob.intervalSamples {
		sum += sample
		if sample < min {
			min = sample
		}
		if sample > max {
			max = sample
		}
	}

	avg := sum / time.Duration(len(rob.intervalSamples))
	jitter := max - min

	// Check if measurements are stable (low jitter)
	maxJitter := avg / 4 // Allow 25% jitter
	if jitter <= maxJitter {
		// Update interval if significantly different or if not yet stable
		if !rob.intervalStable || abs(avg-rob.sampleInterval) > rob.sampleInterval/10 {
			oldInterval := rob.sampleInterval
			rob.sampleInterval = avg
			rob.intervalStable = true

			log.Printf("Interval updated: %v -> %v (jitter: %v, samples: %d)",
				oldInterval, avg, jitter, len(rob.intervalSamples))
		}
	} else {
		// High jitter detected
		if rob.intervalStable {
			log.Printf("High jitter detected (%v), reverting to fallback interval %v",
				jitter, rob.fallbackInterval)
			rob.resetIntervalEstimate()
		}
	}
}

func (rob *SensorROB) resetIntervalEstimate() {
	rob.sampleInterval = rob.fallbackInterval
	rob.intervalSamples = rob.intervalSamples[:0]
	rob.intervalStable = false
	rob.lastSeqTime = time.Time{}
}

func (rob *SensorROB) calibrateBaseTime(firstSeqNum uint, receiveTime time.Time) {
	// Estimate when sequence 0 would have been sampled
	// by working backwards from current sequence number
	estimatedDelay := time.Duration(firstSeqNum) * rob.sampleInterval
	rob.baseSampleTime = receiveTime.Add(-estimatedDelay)

	log.Printf("Calibrated base time: seq=%d, recv=%v, base=%v",
		firstSeqNum, receiveTime, rob.baseSampleTime)

}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func (rob *SensorROB) detectTimeAnomaly(recvTime time.Time) bool {
	if rob.lastReceiveTime.IsZero() {
		return false
	}

	// If receive time goes backwards by more than reasonable network jitter
	maxJitter := 100 * time.Millisecond
	if recvTime.Before(rob.lastReceiveTime.Add(-maxJitter)) {
		return true
	}

	// If receive time jumps forward by more than expected
	maxJump := time.Duration(rob.windowSize) * rob.sampleInterval * 2
	if recvTime.After(rob.lastReceiveTime.Add(maxJump)) {
		return true
	}

	return false
}

// Bitmap implementation (unchanged from original)
type bitmap struct {
	bits []byte
	num  int
}

func newBitmap(num int) *bitmap {
	return &bitmap{
		bits: make([]byte, (num/8)+1),
		num:  num,
	}
}

func (b *bitmap) set(index int) {
	b.bits[index/8] |= 1 << (7 - index%8)
}

func (b *bitmap) isSet(index int) bool {
	return b.bits[index/8]&(1<<(7-index%8)) != 0
}

func (b *bitmap) consume() (consumed int) {
	for idx := 0; idx < b.num; idx++ {
		if !b.isSet(idx) {
			break
		}
		consumed++
	}

	if consumed == 0 {
		return
	}

	// Simplified bit shifting - shift by consumed positions
	byteShift := consumed / 8
	bitShift := consumed % 8

	if byteShift > 0 {
		copy(b.bits, b.bits[byteShift:])
		for i := len(b.bits) - byteShift; i < len(b.bits); i++ {
			b.bits[i] = 0
		}
	}

	if bitShift > 0 {
		for i := 0; i < len(b.bits)-1; i++ {
			b.bits[i] = (b.bits[i] << bitShift) | (b.bits[i+1] >> (8 - bitShift))
		}
		b.bits[len(b.bits)-1] <<= bitShift
	}

	return
}
