package rob

import (
	"errors"
	"time"
)

var (
	// ErrSeqNumOutOfWindow is returned when the sequence number is out of the window.
	ErrSeqNumOutOfWindow = errors.New("sequence number out of window")
	// ErrSeqNumDuplicated is returned when the sequence number is duplicated.
	ErrSeqNumDuplicated = errors.New("sequence number duplicated")
)

type robItem interface {
	SequenceNumber() uint64
	SetLogicalTime(time.Time)
}

type ROBConfig struct {
	OutputChannelSize   int
	WindowSize          uint64
	AuxiliaryBufferSize uint64
	MaxSeqNum           uint64
	FlushTreshold       float64
}

type ROB[T robItem] struct {
	outputCh chan T

	primaryBuf   *buffer[T]
	auxiliaryBuf *buffer[T]

	flushTreshold float64
}

func NewROB[T robItem](cfg *ROBConfig) *ROB[T] {
	return &ROB[T]{
		outputCh: make(chan T, cfg.OutputChannelSize),

		primaryBuf:   newBuffer[T](cfg.WindowSize, 0, cfg.MaxSeqNum),
		auxiliaryBuf: newBuffer[T](cfg.AuxiliaryBufferSize, cfg.WindowSize, cfg.MaxSeqNum),

		flushTreshold: cfg.FlushTreshold,
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
	seqNum := item.SequenceNumber()

	// Check if the sequence number is out of the window
	if !rob.primaryBuf.inRange(seqNum) {
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
	seqNum := item.SequenceNumber()

	// Check if the sequence number is out of the window
	if !rob.auxiliaryBuf.inRange(seqNum) {
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
func (rob *ROB[T]) Enqueue(item T) error {
	err := rob.enqueuePrimary(item)
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrSeqNumDuplicated) {
		return err
	}

	return rob.enqueueAuxiliary(item)
}

func (rob *ROB[T]) GetOutputCh() chan T {
	return rob.outputCh
}
