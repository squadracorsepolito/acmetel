package pool

import "github.com/squadracorsepolito/acmetel/internal/rb"

type fanIn[T any] struct {
	buffer *rb.RingBuffer[T]
}

func newFanIn[T any](bufferCapacity int) *fanIn[T] {
	return &fanIn[T]{
		buffer: rb.NewRingBuffer[T](uint32(bufferCapacity), rb.BufferKindMPMC),
	}
}

func (fi *fanIn[T]) addTask(task T) error {
	return fi.buffer.Write(task)
}

func (fo *fanIn[T]) readTask() (T, error) {
	return fo.buffer.Read()
}

func (fo *fanIn[T]) close() {
	fo.buffer.Close()
}
