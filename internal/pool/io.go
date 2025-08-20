package pool

import (
	"github.com/squadracorsepolito/acmetel/internal/message"
)

type withOutput[T message.Message] struct {
	ch chan T
}

func newWithOutput[T message.Message](chSize int) *withOutput[T] {
	return &withOutput[T]{
		ch: make(chan T, chSize),
	}
}

// GetOutputCh returns the output channel of the worker pool.
// The output channel is used by the worker pool
// to send processed messages.
func (wo *withOutput[T]) GetOutputCh() <-chan T {
	return wo.ch
}

func (wo *withOutput[T]) sendOutput(item T) {
	wo.ch <- item
}

func (wo *withOutput[T]) closeOutput() {
	close(wo.ch)
}
