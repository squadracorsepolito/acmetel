package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_bitmap(t *testing.T) {
	assert := assert.New(t)

	b := newBitmap(20)
	assert.Len(b.bits, 3)

	b.set(0)
	b.set(1)
	b.set(17)
	b.set(18)
	assert.Equal(byte(0b11000000), b.bits[0])
	assert.Equal(byte(0), b.bits[1])
	assert.Equal(byte(0b01100000), b.bits[2])

	assert.True(b.isSet(0))
	assert.True(b.isSet(1))
	assert.True(b.isSet(17))
	assert.True(b.isSet(18))
	assert.False(b.isSet(19))

	consumed := b.consume()
	assert.Equal(uint(2), consumed)
	assert.Equal(byte(0), b.bits[0])
	assert.Equal(byte(0b00000001), b.bits[1])
	assert.Equal(byte(0b10000000), b.bits[2])

	b.bits[0] = 0b11111111
	b.bits[1] = 0b10111111
	b.bits[2] = 0b10100000
	consumed = b.consume()
	assert.Equal(uint(9), consumed)
	assert.Equal(byte(0b01111111), b.bits[0])
	assert.Equal(byte(0b01000000), b.bits[1])
	assert.Equal(byte(0), b.bits[2])

	b.bits[0] = 0b11111111
	b.bits[1] = 0b01010101
	b.bits[2] = 0b01010000
	consumed = b.consume()
	assert.Equal(uint(8), consumed)
	assert.Equal(byte(0b01010101), b.bits[0])
	assert.Equal(byte(0b01010000), b.bits[1])
	assert.Equal(byte(0), b.bits[2])

	b.bits[0] = 0b11111111
	b.bits[1] = 0b11111111
	b.bits[2] = 0b01010000
	consumed = b.consume()
	assert.Equal(uint(16), consumed)
	assert.Equal(byte(0b01010000), b.bits[0])
	assert.Equal(byte(0), b.bits[1])
	assert.Equal(byte(0), b.bits[2])

	b.bits[0] = 0b11111111
	b.bits[1] = 0b11111111
	b.bits[2] = 0b11010000
	consumed = b.consume()
	assert.Equal(uint(18), consumed)
	assert.Equal(byte(0b01000000), b.bits[0])
	assert.Equal(byte(0), b.bits[1])
	assert.Equal(byte(0), b.bits[2])

	b.reset()
	assert.Equal(byte(0), b.bits[0])
	assert.Equal(byte(0), b.bits[1])
	assert.Equal(byte(0), b.bits[2])
}
