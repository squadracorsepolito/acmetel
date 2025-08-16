package udp

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
	"go.opentelemetry.io/otel/attribute"
)

const (
	defaultUDPPayloadSize = 1474
)

type workerArgs struct {
	conn *net.UDPConn
}

type worker struct {
	tel *internal.Telemetry

	conn *net.UDPConn
	buf  []byte

	// Telemetry metrics
	receivedBytes atomic.Int64
}

func (w *worker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *worker) initMetrics() {
	w.tel.NewCounter("received_bytes", func() int64 { return w.receivedBytes.Load() })
}

func (w *worker) Init(ctx context.Context, args *workerArgs) error {
	w.conn = args.conn

	// Hacky method to close the connection when the context is done
	go func() {
		<-ctx.Done()
		w.conn.Close()
	}()

	// Create a buffer for the payload
	w.buf = make([]byte, defaultUDPPayloadSize)

	w.initMetrics()

	return nil
}

func (w *worker) Receive(ctx context.Context) (*Message, bool, error) {
	_, err := w.conn.Read(w.buf)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			select {
			case <-ctx.Done():
				return nil, true, nil

			default:
				w.tel.LogError("failed to read connection", err)
			}

			return nil, true, err
		}

		w.tel.LogError("server failed to read", err)

		return nil, false, err
	}

	_, span := w.tel.NewTrace(ctx, "process UDP frame")
	defer span.End()

	dataLen := len(w.buf)
	data := make([]byte, dataLen)
	copy(data, w.buf)

	res := newMessage(data, dataLen)
	res.SetReceiveTime(time.Now())

	span.SetAttributes(attribute.Int("data_len", dataLen))
	res.SaveSpan(span)

	w.receivedBytes.Add(int64(dataLen))

	return res, false, nil
}

func (w *worker) Close(_ context.Context) error {
	return nil
}
