package egress

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/stage"
	"go.opentelemetry.io/otel/attribute"
)

//////////////
//  CONFIG  //
//////////////

// TCPConfig structs contains the configuration for the TCP egress stage.
type TCPConfig struct {
	PoolConfig *pool.Config

	// IPAddr is the destination IP address.
	//
	// Default: 127.0.0.1
	IPAddr string `yaml:"ip_addr" json:"ip_addr"`

	// Port is the destination port.
	//
	// Default: 20_000
	Port uint16 `yaml:"port" json:"port"`

	// WriteTimeout is the timeout for writing messages to the TCP connection.
	//
	// Default: 10s
	WriteTimeout time.Duration `yaml:"write_timeout" json:"write_timeout"`
}

// DefaultTCPConfig returns a default TCPConfig.
func DefaultTCPConfig() *TCPConfig {
	return &TCPConfig{
		PoolConfig:   pool.DefaultConfig(),
		IPAddr:       "127.0.0.1",
		Port:         20_000,
		WriteTimeout: 10 * time.Second,
	}
}

//////////////
//  WORKER  //
//////////////

type tcpWorkerArgs struct {
	conn         *net.TCPConn
	writeTimeout time.Duration
}

func newTCPWorkerArgs(conn *net.TCPConn, writeTimeout time.Duration) *tcpWorkerArgs {
	return &tcpWorkerArgs{
		conn:         conn,
		writeTimeout: writeTimeout,
	}
}

type tcpWorker[T msgSer] struct {
	pool.BaseWorker

	conn         *net.TCPConn
	writeTimeout time.Duration

	// Metrics
	deliveredBytes atomic.Int64
}

func (tw *tcpWorker[T]) Init(_ context.Context, args *tcpWorkerArgs) error {
	tw.conn = args.conn
	tw.writeTimeout = args.writeTimeout

	tw.initMetrics()

	return nil
}

func (tw *tcpWorker[T]) initMetrics() {
	tw.Tel.NewCounter("delivered_bytes", func() int64 { return tw.deliveredBytes.Load() })
}

func (tw *tcpWorker[T]) Deliver(ctx context.Context, msg T) error {
	// Extract the span context from the input message
	_, span := tw.Tel.NewTrace(msg.LoadSpanContext(ctx), "deliver TCP message")
	defer span.End()

	// Set the write timeout
	if err := tw.conn.SetWriteDeadline(time.Now().Add(tw.writeTimeout)); err != nil {
		return err
	}

	tcpMsg := msg.GetBytes()
	deliveredBytes, err := tw.conn.Write(tcpMsg)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.Int("message_size", len(tcpMsg)))

	// Update metrics
	tw.deliveredBytes.Add(int64(deliveredBytes))

	return nil
}

func (tw *tcpWorker[T]) Close(_ context.Context) error {
	return nil
}

/////////////
//  STAGE  //
/////////////

// TCPStage is an egress stage that writes messages to a TCP connection.
type TCPStage[T msgSer] struct {
	*stage.Egress[T, tcpWorker[T], *tcpWorkerArgs, *tcpWorker[T]]

	cfg *TCPConfig

	conn *net.TCPConn
}

// NewTCPStage returns a new TCP egress stage.
func NewTCPStage[T msgSer](inputConnector conn[T], cfg *TCPConfig) *TCPStage[T] {
	return &TCPStage[T]{
		Egress: stage.NewEgress[T, tcpWorker[T], *tcpWorkerArgs](
			"tcp", inputConnector, cfg.PoolConfig,
		),

		cfg: cfg,
	}
}

// Init initializes the stage.
func (ts *TCPStage[T]) Init(ctx context.Context) error {
	// Parse the IP address
	parsedAddr, err := netip.ParseAddr(ts.cfg.IPAddr)
	if err != nil {
		return err
	}
	addr := net.TCPAddrFromAddrPort(netip.AddrPortFrom(parsedAddr, ts.cfg.Port))

	// Dial the TCP connection
	conn, err := net.DialTCP("tcp", nil, addr)
	if err != nil {
		return err
	}

	ts.conn = conn

	return ts.Egress.Init(ctx, newTCPWorkerArgs(ts.conn, ts.cfg.WriteTimeout))
}
