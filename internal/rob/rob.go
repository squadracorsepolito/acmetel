// Package rob implements a re-order buffer.
package rob

import (
	"errors"
)

var (
	// ErrSeqNumOutOfWindow is returned when the sequence number is out of the window.
	ErrSeqNumOutOfWindow = errors.New("sequence number out of window")
	// ErrSeqNumDuplicated is returned when the sequence number is duplicated.
	ErrSeqNumDuplicated = errors.New("sequence number duplicated")
	// ErrSeqNumTooBig is returned when the sequence number is too big.
	ErrSeqNumTooBig = errors.New("sequence number too big")
)

// Config is the configuration for the re-order buffer structure [ROB].
type Config struct {
	// OutputChannelSize is the size of the output channel.
	OutputChannelSize int

	// MaxSeqNum is the maximum possible sequence number.
	MaxSeqNum uint64

	// PrimaryBufferSize is the size of the primary buffer.
	PrimaryBufferSize uint64

	// AuxiliaryBufferSize is the size of the auxiliary buffer.
	AuxiliaryBufferSize uint64

	// FlushTreshold is the value of the fullness of the auxiliary buffer
	// needed for flushing the primary buffer.
	FlushTreshold float64

	// BaseAlpha is the base value for the alpha parameter for the EMA.
	BaseAlpha float64

	// JumpThreshold is the threshold used by the time smoother (EMA)
	// for adjusting the alpha parameter when there is a jump in the sequence.
	JumpThreshold uint64
}

type robItem interface {
	bufferItem
	timeSmootherItem
}

// ROB is an implementation of a re-order buffer.
// It uses two buffers, a primary and an auxiliary.
// The primary is automatically flushed when the auxiliary fullness
// is higher than the flush treshold.
// It uses the EMA (exponential moving average) technique to smooth and adjust
// the time associated with an item.
type ROB[T robItem] struct {
	outputCh chan T

	primaryBuf   *buffer[T]
	auxiliaryBuf *buffer[T]

	flushTreshold float64

	timeSmoother *timeSmoother[T]

	isInitialized bool
}

// NewROB returns a new [ROB] (re-order buffer) with the given configuration.
func NewROB[T robItem](cfg *Config) *ROB[T] {
	return &ROB[T]{
		outputCh: make(chan T, cfg.OutputChannelSize),

		primaryBuf:   newBuffer[T](cfg.PrimaryBufferSize, 0, cfg.MaxSeqNum),
		auxiliaryBuf: newBuffer[T](cfg.AuxiliaryBufferSize, cfg.PrimaryBufferSize, cfg.MaxSeqNum),

		flushTreshold: cfg.FlushTreshold,

		timeSmoother: newTimeSmoother[T](cfg.BaseAlpha, cfg.JumpThreshold, cfg.MaxSeqNum),

		isInitialized: false,
	}
}

func (rob *ROB[T]) tryDequeueFromPrimary() {
	deqItems := rob.primaryBuf.dequeueConsecutives()
	deqItemCount := uint64(len(deqItems))

	if deqItemCount == 0 {
		return
	}

	for _, tmpItem := range deqItems {
		rob.deliver(tmpItem)
	}

	rob.auxiliaryBuf.transfer(rob.primaryBuf, deqItemCount)
}

func (rob *ROB[T]) enqueuePrimary(item T) error {
	seqNum := item.GetSequenceNumber()

	// Check if the sequence number is out of the window
	if !rob.primaryBuf.isInRange(seqNum) {
		return ErrSeqNumOutOfWindow
	}

	// Check if the sequence number is duplicated
	if rob.primaryBuf.isDuplicated(seqNum) {
		return ErrSeqNumDuplicated
	}

	// Enqueue the item with the skip flag
	if !rob.primaryBuf.enqueue(item, true) {
		// The item is the next and the buffer is empty,
		// so deliver it directly and transfer the first item
		// of the auxiliary buffer into the primary
		rob.auxiliaryBuf.transfer(rob.primaryBuf, 1)
		rob.deliver(item)
		return nil
	}

	// Dequeue and deliver consecutive items
	deqItems := rob.primaryBuf.dequeueConsecutives()

	for _, tmpItem := range deqItems {
		rob.deliver(tmpItem)
	}

	// Transfer the delivered amount of items from the auxiliary buffer
	// to the primary
	deqItemCount := uint64(len(deqItems))
	rob.auxiliaryBuf.transfer(rob.primaryBuf, deqItemCount)

	// If the delivered item count matches the window size,
	// try to dequeue consecutive items from the primary buffer
	if deqItemCount == rob.primaryBuf.size {
		rob.tryDequeueFromPrimary()
	}

	return nil
}

func (rob *ROB[T]) enqueueAuxiliary(item T) error {
	seqNum := item.GetSequenceNumber()

	// Check if the sequence number is out of the window
	if !rob.auxiliaryBuf.isInRange(seqNum) {
		return ErrSeqNumOutOfWindow
	}

	// Check if the sequence number is duplicated
	if rob.auxiliaryBuf.isDuplicated(seqNum) {
		return ErrSeqNumDuplicated
	}

	// Always enqueue the item
	rob.auxiliaryBuf.enqueue(item, false)

	// If the auxiliary buffer is full at more than the flush treshold,
	// flush the first buffer and transfer the items
	// from the auxiliary buffer to the primary
	if rob.auxiliaryBuf.getFullness() > rob.flushTreshold {
		for _, tmpItem := range rob.primaryBuf.flush() {
			rob.deliver(tmpItem)
		}

		// Transfer the items from the auxiliary buffer
		// to the primary
		rob.primaryBuf.transfer(rob.auxiliaryBuf, rob.primaryBuf.size)

		// Try to dequeue consecutive items from the primary buffer
		rob.tryDequeueFromPrimary()
	}

	return nil
}

func (rob *ROB[T]) deliver(item T) {
	rob.timeSmoother.adjust(item)
	rob.outputCh <- item
}

// Enqueue tries to add the item into the ROB.
// If the item is in sequence, it sends it to the output channel.
// Otherwise, it tries to add the item into the primary or the auxiliary buffer.
//
// It returns:
//   - [ErrSeqNumOutOfWindow] if the sequence number is out of the window
//     for both the buffers
//   - [ErrSeqNumDuplicated] if the sequence number is duplicated
//   - [ErrSeqNumTooBig] if the sequence number is too big
func (rob *ROB[T]) Enqueue(item T) error {
	seqNum := item.GetSequenceNumber()

	if !rob.primaryBuf.isValidSize(seqNum) {
		return ErrSeqNumTooBig
	}

	if !rob.isInitialized {
		rob.primaryBuf.setStartSeqNum(seqNum)

		rob.auxiliaryBuf.setStartSeqNum(seqNum)
		rob.auxiliaryBuf.incrementNextSeqNum(rob.primaryBuf.size)

		rob.isInitialized = true
	}

	err := rob.enqueuePrimary(item)
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrSeqNumDuplicated) {
		return err
	}

	return rob.enqueueAuxiliary(item)
}

// GetOutputCh returns the output channel of the re-order buffer.
func (rob *ROB[T]) GetOutputCh() chan T {
	return rob.outputCh
}

func (rob *ROB[T]) reset() {
	rob.primaryBuf.reset()
	rob.auxiliaryBuf.reset()
	rob.timeSmoother.reset()
	rob.isInitialized = false
}

// FlushAndReset flushes the ROB and resets all the parameters.
func (rob *ROB[T]) FlushAndReset() {
	for _, item := range rob.primaryBuf.flush() {
		rob.deliver(item)
	}

	for _, item := range rob.auxiliaryBuf.flush() {
		rob.deliver(item)
	}

	rob.reset()
}
