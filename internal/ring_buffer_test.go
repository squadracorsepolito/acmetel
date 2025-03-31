package internal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_RingBuffer_Put(t *testing.T) {
	assert := assert.New(t)

	ctx := context.Background()

	rb := NewRingBuffer[[]byte](8)

	str1 := []byte("hello")
	itemIn := make([]byte, 8)
	for idx, ch := range str1 {
		itemIn[idx] = ch
	}
	rb.Put(ctx, itemIn)

	str2 := []byte("world")
	itemIn = make([]byte, 8)
	for idx, ch := range str2 {
		itemIn[idx] = ch
	}
	rb.Put(ctx, itemIn)

	itemOut, _ := rb.Get(ctx)
	for idx, ch := range str1 {
		assert.Equal(ch, itemOut[idx])
	}

	itemOut, _ = rb.Get(ctx)
	for idx, ch := range str2 {
		assert.Equal(ch, itemOut[idx])
	}
}

func Benchmark_RingBuffer(b *testing.B) {
	b.ReportAllocs()

	ctx := context.Background()

	rb := NewRingBuffer[[]byte](2048)
	item := make([]byte, 2048)

	for b.Loop() {
		item[0] = 'h'
		item[1] = 'e'
		item[2] = 'l'
		item[3] = 'l'
		item[4] = 'o'
		rb.Put(ctx, item)

		rb.Get(ctx)
	}
}
