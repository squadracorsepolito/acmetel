package ingress

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/internal/message"
	"go.opentelemetry.io/otel/attribute"
)

const (
	tcpBufferSize = 4096
)

//////////////
//  CONFIG  //
//////////////

///////////////
//  MESSAGE  //
///////////////

type TCPMessage struct {
	message.Base

	Payload     []byte
	PayloadSize int
}

func newTCPMessage() *TCPMessage {
	return &TCPMessage{}
}

//////////////
//  SOURCE  //
//////////////

type tcpSource struct {
	tel *internal.Telemetry

	wg *sync.WaitGroup

	bufPool sync.Pool

	listener *net.TCPListener

	// Configs
	delimiter    []byte
	delimiterLen int
	timeout      time.Duration

	// Metrics
	openConnections  atomic.Int64
	receivedBytes    atomic.Int64
	receivedMessages atomic.Int64
}

func newTCPSource() *tcpSource {
	return &tcpSource{
		wg: &sync.WaitGroup{},

		bufPool: sync.Pool{
			New: func() any {
				buf := make([]byte, tcpBufferSize)
				return buf
			},
		},
	}
}

func (ts *tcpSource) SetTelemetry(tel *internal.Telemetry) {
	ts.tel = tel
}

func (ts *tcpSource) init(ipAddr string, port uint16, delimiter []byte, timeout time.Duration) error {
	parsedAddr, err := netip.ParseAddr(ipAddr)
	if err != nil {
		return err
	}

	addr := netip.AddrPortFrom(parsedAddr, port)
	listener, err := net.ListenTCP("tcp", net.TCPAddrFromAddrPort(addr))
	if err != nil {
		return err
	}

	ts.listener = listener

	ts.delimiter = delimiter
	ts.delimiterLen = len(delimiter)
	ts.timeout = timeout

	ts.initMetrics()

	return nil
}

func (ts *tcpSource) initMetrics() {
	ts.tel.NewUpDownCounter("open_connections", func() int64 { return ts.openConnections.Load() })
	ts.tel.NewCounter("received_bytes", func() int64 { return ts.receivedBytes.Load() })
	ts.tel.NewCounter("received_messages", func() int64 { return ts.receivedMessages.Load() })
}

func (ts *tcpSource) Run(ctx context.Context, outConnector conn[*TCPMessage]) {
	// Close the listener when the context is done
	go func() {
		<-ctx.Done()
		ts.listener.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn, err := ts.listener.Accept()
		if err != nil {
			// Check if the error is because the context is done
			select {
			case <-ctx.Done():
				ts.wg.Wait()
				return

			default:
				ts.tel.LogError("failed to accept connection", err)
				continue
			}
		}

		// Spawn a goroutine to handle the connection
		ts.wg.Add(1)
		go ts.handleConn(ctx, conn, outConnector)
	}
}

func (ts *tcpSource) handleConn(ctx context.Context, conn net.Conn, outConnector conn[*TCPMessage]) {
	defer ts.wg.Done()

	// Handle the open connections metric
	ts.openConnections.Add(1)
	defer ts.openConnections.Add(-1)

	// Channel to notify when the connection is closed normally
	normallyClosed := make(chan struct{})
	defer close(normallyClosed)

	// Close the connection when the context is done
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-normallyClosed:
			// Connection closed normally
		}
	}()

	// Get the buffer from the pool
	buf := ts.bufPool.Get().([]byte)
	defer ts.bufPool.Put(buf)

	var acc []byte

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set the read deadline
		conn.SetReadDeadline(time.Now().Add(ts.timeout))

		// Read the TCP stream
		n, err := conn.Read(buf)
		if err != nil {
			// Check if the connection has been closed by the client,
			// if so, close the server connection
			if errors.Is(err, io.EOF) {
				goto closeConnection
			}

			// Check if the connection is closed and if the context is done
			// return without re-closing the connection
			if errors.Is(err, net.ErrClosed) {
				select {
				case <-ctx.Done():
					return
				default:
				}
			}

			// For any other error, break the loop and close the server connection.
			// This is likely be caused by the read deadline being exceeded.
			ts.tel.LogError("failed to read connection", err)
			goto closeConnection
		}

		// Append the new bytes to the accumulator
		acc = append(acc, buf[:n]...)

		// If the accumulator is smaller than the delimiter,
		// continue reading the TCP stream
		if len(acc) < ts.delimiterLen {
			continue
		}

		for {
			idx := bytes.Index(acc, ts.delimiter)
			// If the delimiter is not found, break the loop
			// and continue reading the TCP stream
			if idx == -1 {
				break
			}

			// Extract the message without delimiter
			msg := acc[:idx]

			// Handle the message and send the result to the output connector
			if err := outConnector.Write(ts.handleMessage(ctx, msg)); err != nil {
				ts.tel.LogError("failed to write message to output connector", err)
			}

			// Remove the message from the accumulator
			acc = acc[idx+ts.delimiterLen:]
		}

		// Prevent accumulator from growing too large
		if len(acc) > 1024*1024 { // 1MB limit
			ts.tel.LogWarn("message too large, closing connection")
			goto closeConnection
		}
	}

closeConnection:
	conn.Close()
	normallyClosed <- struct{}{}
}

func (ts *tcpSource) handleMessage(ctx context.Context, msg []byte) *TCPMessage {
	// Create the trace for the incoming message
	_, span := ts.tel.NewTrace(ctx, "receive TCP message")
	defer span.End()

	// Create the TCP message
	tcpMsg := newTCPMessage()

	// Extract the payload from the buffer
	payloadSize := len(msg)
	tcpMsg.PayloadSize = payloadSize
	tcpMsg.Payload = make([]byte, payloadSize)
	copy(tcpMsg.Payload, msg)

	// Set the receive time and the timestamp
	recvTime := time.Now()
	tcpMsg.SetReceiveTime(recvTime)
	tcpMsg.SetTimestamp(recvTime)

	// Save the span into the message
	span.SetAttributes(attribute.Int("payload_size", payloadSize))
	tcpMsg.SaveSpan(span)

	// Update metrics
	ts.receivedBytes.Add(int64(payloadSize))
	ts.receivedMessages.Add(1)

	return tcpMsg
}

/////////////
//  STAGE  //
/////////////
