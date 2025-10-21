package ingress

import (
	"bytes"
	"context"
	"encoding/binary"
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
	tcpBufSize = 4096
)

//////////////
//  CONFIG  //
//////////////

// Endianess defines the endianness of a slice of bytes.
type Endianess uint8

const (
	// LittleEndian defines little endianess.
	LittleEndian Endianess = iota
	// BigEndian defines big endianess.
	BigEndian
)

// TCPFramingMode defines the framing mode to use.
type TCPFramingMode uint8

const (
	// TCPFramingModeDelimited will use delimited messages.
	TCPFramingModeDelimited TCPFramingMode = iota
	// TCPFramingModeLengthPrefixed will use length-prefixed messages.
	TCPFramingModeLengthPrefixed
)

// TCPConfig structs contains the configuration for the TCP ingress stage.
type TCPConfig struct {
	// IPAddr is the IP address of the server to listen on.
	//
	// Default: 0.0.0.0
	IPAddr string `yaml:"ip_addr" json:"ip_addr"`

	// Port is the port to listen on.
	//
	// Default: 20_000
	Port uint16 `yaml:"port" json:"port"`

	// ReadTimeout is the timeout for reading from a connection.
	//
	// Default: 10s
	ReadTimeout time.Duration `yaml:"read_timeout" json:"read_timeout"`

	// FramingMode is the framing mode to use.
	// It basically defines how the messages are separated.
	//
	// Default: TCPFramingModeDelimited
	FramingMode TCPFramingMode

	// MaxMessageSize is the maximum size of a message.
	// If the accumulator that is holding the message
	// gets bigger, the connection is closed.
	//
	// Default: 4MB
	MaxMessageSize int

	// Delimiter is the delimiter to use to separate messages
	// when the FramingMode is TCPFramingModeDelimited.
	//
	// Default: "\r\n"
	Delimiter []byte `yaml:"delimiter" json:"delimiter"`

	// HeaderLen is the length of the header in the context
	// of the TCPFramingModeLengthPrefixed mode.
	HeaderLen int

	// MessageLengthFieldOffset is the offset in the header
	// of the message length field when FramingMode is TCPFramingModeLengthPrefixed.
	MessageLengthFieldOffset int

	// MessageLengthFieldLen is the length of the message length field
	// when FramingMode is TCPFramingModeLengthPrefixed.
	MessageLengthFieldLen int

	// MessageLengthFieldEndianess is the endianess (byte order)
	// of the message length field when FramingMode is TCPFramingModeLengthPrefixed.
	MessageLengthFieldEndianess Endianess
}

// DefaultTCPConfig returns a default TCPConfig.
func DefaultTCPConfig() TCPConfig {
	return TCPConfig{
		IPAddr:         "0.0.0.0",
		Port:           20_000,
		ReadTimeout:    10 * time.Second,
		FramingMode:    TCPFramingModeDelimited,
		MaxMessageSize: 4 * 1024 * 1024,
		Delimiter:      []byte("\r\n"),
	}
}

///////////////
//  MESSAGE  //
///////////////

var _ message.Serializable = (*TCPMessage)(nil)

// TCPMessage represents a TCP message.
type TCPMessage struct {
	message.Base

	// RemoteAddr is the remote address of the connection.
	RemoteAddr string
	// Message is the message payload.
	Message []byte
	// MessageSize is the size of the message payload.
	MessageSize int
}

func newTCPMessage() *TCPMessage {
	return &TCPMessage{}
}

// GetBytes returns the bytes of the TCP message.
func (tm *TCPMessage) GetBytes() []byte {
	return tm.Message
}

//////////////
//  SOURCE  //
//////////////

type tcpSourceConfig struct {
	readTimeout time.Duration

	framingMode TCPFramingMode
	maxMsgSize  int

	delimiter []byte

	headerLen            int
	msgLenFieldOffset    int
	msgLenFieldLen       int
	msgLenFieldEndianess Endianess
}

type tcpSource struct {
	tel *internal.Telemetry

	wg *sync.WaitGroup

	bufPool sync.Pool

	listener *net.TCPListener

	readTimeout time.Duration

	// Framing
	framingMode TCPFramingMode
	maxMsgSize  int
	// Delimited
	delimiter    []byte
	delimiterLen int
	// Lenght Prefixed
	headerLen            int
	msgLenFieldOffset    int
	msgLenFieldLen       int
	msgLenFieldParseLen  int
	msgLenFieldEndianess Endianess

	// Metrics
	openConnections  atomic.Int64
	receivedBytes    atomic.Int64
	receivedMessages atomic.Int64
}

func newTCPSource(cfg *tcpSourceConfig) *tcpSource {
	msgLenFieldParseLen := cfg.msgLenFieldLen
	switch msgLenFieldParseLen {
	case 3:
		msgLenFieldParseLen = 4
	case 5, 6, 7:
		msgLenFieldParseLen = 8
	}

	return &tcpSource{
		wg: &sync.WaitGroup{},

		bufPool: sync.Pool{
			New: func() any {
				buf := make([]byte, tcpBufSize)
				return buf
			},
		},

		readTimeout: cfg.readTimeout,

		framingMode: cfg.framingMode,
		maxMsgSize:  cfg.maxMsgSize,

		headerLen:            cfg.headerLen,
		msgLenFieldOffset:    cfg.msgLenFieldOffset,
		msgLenFieldLen:       cfg.msgLenFieldLen,
		msgLenFieldParseLen:  msgLenFieldParseLen,
		msgLenFieldEndianess: cfg.msgLenFieldEndianess,

		delimiter:    cfg.delimiter,
		delimiterLen: len(cfg.delimiter),
	}
}

func (ts *tcpSource) SetTelemetry(tel *internal.Telemetry) {
	ts.tel = tel
}

func (ts *tcpSource) init(ipAddr string, port uint16) error {
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
	defer conn.Close()

	// Channel to notify when the connection is closed normally
	connClosed := make(chan struct{})
	defer close(connClosed)

	// Close the connection when the context is done
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-connClosed:
			// Connection closed normally
		}
	}()

	// Handle the open connections metric
	ts.openConnections.Add(1)
	defer ts.openConnections.Add(-1)

	// Get the buffer from the pool
	buf := ts.bufPool.Get().([]byte)
	defer ts.bufPool.Put(buf)

	// Preallocate the accumulator
	accBaseCap := 4 * tcpBufSize
	acc := make([]byte, 0, accBaseCap)

	minAccLen := 0
	switch ts.framingMode {
	case TCPFramingModeDelimited:
		minAccLen = ts.delimiterLen
	case TCPFramingModeLengthPrefixed:
		minAccLen = ts.headerLen
	}

loop:
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set the read deadline
		conn.SetReadDeadline(time.Now().Add(ts.readTimeout))

		// Read the TCP stream
		n, err := conn.Read(buf)
		if err != nil {
			// Check if the connection has been closed by the client,
			// if so, close the server connection
			if errors.Is(err, io.EOF) {
				return
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
			return
		}

		// Append the new bytes to the accumulator
		acc = append(acc, buf[:n]...)

		for {
			accLen := len(acc)

			// If the accumulator is smaller than the minimum length,
			// continue reading the TCP stream
			if accLen < minAccLen {
				continue loop
			}

			// Get the length of the message.
			msgLen := 0
			totLen := 0
			switch ts.framingMode {
			case TCPFramingModeDelimited:
				msgLen = bytes.Index(acc, ts.delimiter)
				totLen = msgLen + ts.delimiterLen

			case TCPFramingModeLengthPrefixed:
				msgLen = ts.parseHeader(acc[:ts.headerLen])
				totLen = msgLen + ts.headerLen
			}

			if msgLen == -1 || accLen < totLen {
				// If the message length is not found or the accumulator is too small,
				// break the loop and continue reading the TCP stream
				break
			}

			// Extract the message
			msg := acc[:totLen]

			// Handle the message and send the result to the output connector
			outMsg := ts.handleMessage(ctx, msg)
			outMsg.RemoteAddr = conn.RemoteAddr().String()
			if err := outConnector.Write(outMsg); err != nil {
				ts.tel.LogError("failed to write message to output connector", err)
			}

			// Remove the message from the accumulator
			acc = acc[totLen:]

			// Check if the accumulator should be reset
			if len(acc) == 0 && cap(acc) > accBaseCap {
				acc = make([]byte, 0, accBaseCap)
				break
			}
		}

		// Prevent accumulator from growing too large
		if len(acc) > ts.maxMsgSize {
			ts.tel.LogWarn("message too large, closing connection")
			return
		}
	}
}

func (ts *tcpSource) parseHeader(header []byte) int {
	if len(header) < ts.headerLen {
		return -1
	}

	msgLenField := header[ts.msgLenFieldOffset : ts.msgLenFieldOffset+ts.msgLenFieldLen]

	buf := msgLenField
	// Check if the message length field should be extended
	if ts.msgLenFieldLen != ts.msgLenFieldParseLen {
		buf = make([]byte, ts.msgLenFieldParseLen)

		switch ts.msgLenFieldEndianess {
		case LittleEndian:
			copy(buf, msgLenField)
		case BigEndian:
			copy(buf[ts.msgLenFieldParseLen-ts.msgLenFieldLen:], msgLenField)
		}
	}

	switch ts.msgLenFieldEndianess {
	case LittleEndian:
		return ts.parseLittleEndianMsgLen(buf)
	case BigEndian:
		return ts.parseBigEndianMsgLen(buf)
	}

	return 0
}

func (ts *tcpSource) parseLittleEndianMsgLen(buf []byte) int {
	switch len(buf) {
	case 1:
		return int(buf[0])
	case 2:
		return int(binary.LittleEndian.Uint16(buf))
	case 4:
		return int(binary.LittleEndian.Uint32(buf))
	case 8:
		return int(binary.LittleEndian.Uint64(buf))
	default:
		return -1
	}
}

func (ts *tcpSource) parseBigEndianMsgLen(buf []byte) int {
	switch len(buf) {
	case 1:
		return int(buf[0])
	case 2:
		return int(binary.BigEndian.Uint16(buf))
	case 4:
		return int(binary.BigEndian.Uint32(buf))
	case 8:
		return int(binary.BigEndian.Uint64(buf))
	default:
		return -1
	}
}

func (ts *tcpSource) handleMessage(ctx context.Context, msg []byte) *TCPMessage {
	// Create the trace for the incoming message
	_, span := ts.tel.NewTrace(ctx, "receive TCP message")
	defer span.End()

	// Create the TCP message
	tcpMsg := newTCPMessage()

	// Extract the payload from the buffer
	msgSize := len(msg)
	tcpMsg.MessageSize = msgSize
	tcpMsg.Message = make([]byte, msgSize)
	copy(tcpMsg.Message, msg)

	// Set the receive time and the timestamp
	recvTime := time.Now()
	tcpMsg.SetReceiveTime(recvTime)
	tcpMsg.SetTimestamp(recvTime)

	// Save the span into the message
	span.SetAttributes(attribute.Int("payload_size", msgSize))
	tcpMsg.SaveSpan(span)

	// Update metrics
	ts.receivedBytes.Add(int64(msgSize))
	ts.receivedMessages.Add(1)

	return tcpMsg
}

/////////////
//  STAGE  //
/////////////

// TCPStage is an ingress stage that reads TCP connections and extracts messages.
type TCPStage struct {
	*stage[*TCPMessage]

	cfg *TCPConfig

	source *tcpSource
}

// NewTCPStage returns a new TCP stage.
func NewTCPStage(outputConnector conn[*TCPMessage], cfg *TCPConfig) *TCPStage {
	source := newTCPSource(&tcpSourceConfig{
		readTimeout:          cfg.ReadTimeout,
		framingMode:          cfg.FramingMode,
		maxMsgSize:           cfg.MaxMessageSize,
		delimiter:            cfg.Delimiter,
		headerLen:            cfg.HeaderLen,
		msgLenFieldOffset:    cfg.MessageLengthFieldOffset,
		msgLenFieldLen:       cfg.MessageLengthFieldLen,
		msgLenFieldEndianess: cfg.MessageLengthFieldEndianess,
	})

	return &TCPStage{
		stage: newStage("tcp", source, outputConnector),

		cfg: cfg,

		source: source,
	}
}

// Init initializes the stage.
func (ts *TCPStage) Init(ctx context.Context) error {
	if err := ts.source.init(ts.cfg.IPAddr, ts.cfg.Port); err != nil {
		return err
	}

	return ts.stage.Init(ctx)
}
