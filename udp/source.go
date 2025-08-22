package udp

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/stage"
	"go.opentelemetry.io/otel/attribute"
)

const (
	defaultUDPPayloadSize = 1474
)

var _ stage.Source[*Message] = (*source)(nil)

type source struct {
	tel *internal.Telemetry

	conn *net.UDPConn

	// Telemetry metrics
	receivedBytes atomic.Int64
}

func newSource() *source {
	return &source{}
}

func (s *source) SetTelemetry(tel *internal.Telemetry) {
	s.tel = tel
}

func (s *source) init(ipAddr string, port uint16) error {
	parsedAddr, err := netip.ParseAddr(ipAddr)
	if err != nil {
		return err
	}

	addr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(parsedAddr, port))
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	s.conn = conn

	s.initMetrics()

	return nil
}

func (s *source) initMetrics() {
	s.tel.NewCounter("received_bytes", func() int64 { return s.receivedBytes.Load() })
}

func (s *source) Run(ctx context.Context, out chan<- *Message) {
	// Hacky method to close the connection when the context is done
	go func() {
		<-ctx.Done()
		s.conn.Close()
	}()

	buf := make([]byte, defaultUDPPayloadSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// read the UDP payload
		_, err := s.conn.Read(buf)
		if err != nil {
			// Check if the connection is closed
			if errors.Is(err, net.ErrClosed) {
				select {
				case <-ctx.Done():
					return

				default:
					s.tel.LogError("failed to read connection", err)
				}

				return
			}

			s.tel.LogError("server failed to read", err)

			return
		}

		// Handle the buffer and send the message
		out <- s.handleBuf(ctx, buf)
	}
}

func (s *source) handleBuf(ctx context.Context, buf []byte) *Message {
	// Create the trace for the incoming datagram
	_, span := s.tel.NewTrace(ctx, "receive UDP datagram")
	defer span.End()

	// Extract the payload from the buffer
	payloadSize := len(buf)
	payload := make([]byte, payloadSize)
	copy(payload, buf)

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
	s.receivedBytes.Add(int64(payloadSize))

	return udpMsg
}
