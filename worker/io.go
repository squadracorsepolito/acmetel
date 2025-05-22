package worker

import (
	"github.com/squadracorsepolito/acmetel/message"
)

type withOutput[T message.Traceable] struct {
	ch chan T
}

func newWithOutput[T message.Traceable](chSize int) *withOutput[T] {
	return &withOutput[T]{
		ch: make(chan T, chSize),
	}
}

func (wo *withOutput[T]) GetOutputCh() <-chan T {
	return wo.ch
}

func (wo *withOutput[T]) sendOutput(item T) {
	wo.ch <- item
}

func (wo *withOutput[T]) closeOutput() {
	close(wo.ch)
}
