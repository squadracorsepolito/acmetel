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
	// read the UDP payload
	_, err := w.conn.Read(w.buf)
	if err != nil {
		// Check if the connection is closed
		if errors.Is(err, net.ErrClosed) {
			select {
			case <-ctx.Done():
				// If the context is done, signal
				// the worker pool to exit the run loop
				return nil, true, nil

			default:
				w.tel.LogError("failed to read connection", err)
			}

			return nil, true, err
		}

		w.tel.LogError("server failed to read", err)

		return nil, false, err
	}

	// Create the trace for the incoming datagram
	_, span := w.tel.NewTrace(ctx, "receive UDP datagram")
	defer span.End()

	// Extract the payload from the buffer
	payloadSize := len(w.buf)
	payload := make([]byte, payloadSize)
	copy(payload, w.buf)

	// Create the UDP message
	udpMsg := newMessage(payload, payloadSize)

	// Set the receive time and the timestamp
	recvTime := time.Now()
	udpMsg.SetReceiveTime(recvTime)
	udpMsg.SetTimestamp(recvTime)

	// Save the span into the message
	span.SetAttributes(attribute.Int("payload_size", payloadSize))
	udpMsg.SaveSpan(span)

	// Update metrics
	w.receivedBytes.Add(int64(payloadSize))

	return udpMsg, false, nil
}

func (w *worker) Close(_ context.Context) error {
	return nil
}
