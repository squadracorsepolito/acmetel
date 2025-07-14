package can

import (
	"context"

	"github.com/squadracorsepolito/acmelib"
)

type decoder struct {
	m map[uint32]func([]byte) []*acmelib.SignalDecoding
}

func newDecoder(messages []*acmelib.Message) *decoder {
	m := make(map[uint32]func([]byte) []*acmelib.SignalDecoding)

	for _, msg := range messages {
		m[uint32(msg.GetCANID())] = msg.SignalLayout().Decode
	}

	return &decoder{
		m: m,
	}
}

func (d *decoder) decode(ctx context.Context, canID uint32, data []byte) []*acmelib.SignalDecoding {
	select {
	case <-ctx.Done():
		return nil
	default:
	}

	fn, ok := d.m[canID]
	if !ok {
		return nil
	}
	return fn(data)
}
