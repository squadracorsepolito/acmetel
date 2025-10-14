package egress

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/internal/pool"
	"github.com/squadracorsepolito/acmetel/internal/stage"
	"go.opentelemetry.io/otel/attribute"
)

//////////////
//  CONFIG  //
//////////////

// UDPConfig structs contains the configuration for the UDP egress stage.
type UDPConfig struct {
	PoolConfig *pool.Config

	// IPAddr is the destination IP address.
	//
	// Default: 127.0.0.1
	IPAddr string `yaml:"ip_addr" json:"ip_addr"`

	// Port is the destination port.
	//
	// Default: 20_000
	Port uint16 `yaml:"port" json:"port"`
}

// DefaultUDPConfig returns the default configuration for the UDP egress stage.
func DefaultUDPConfig() *UDPConfig {
	return &UDPConfig{
		PoolConfig: pool.DefaultConfig(),
		IPAddr:     "127.0.0.1",
		Port:       20_000,
	}
}

//////////////
//  WORKER  //
//////////////

type udpWorkerArgs struct {
	conn *net.UDPConn
}

func newUDPWorkerArgs(conn *net.UDPConn) *udpWorkerArgs {
	return &udpWorkerArgs{
		conn: conn,
	}
}

type udpWorker[T msgSer] struct {
	pool.BaseWorker

	conn *net.UDPConn

	// Metrics
	deliveredBytes atomic.Int64
}

func (uw *udpWorker[T]) initMetrics() {
	uw.Tel.NewCounter("delivered_bytes", func() int64 { return uw.deliveredBytes.Load() })
}

func (uw *udpWorker[T]) Init(_ context.Context, args *udpWorkerArgs) error {
	uw.conn = args.conn

	uw.initMetrics()

	return nil
}

func (uw *udpWorker[T]) Deliver(ctx context.Context, udpMsg T) error {
	// Extract the span context from the input message
	_, span := uw.Tel.NewTrace(udpMsg.LoadSpanContext(ctx), "deliver UDP message")
	defer span.End()

	payload := udpMsg.GetBytes()
	payloadSize := len(payload)

	deliveredBytes, err := uw.conn.Write(payload)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.Int("payload_size", payloadSize))

	// Update metrics
	uw.deliveredBytes.Add(int64(deliveredBytes))

	return nil
}

func (uw *udpWorker[T]) Close(_ context.Context) error {
	return nil
}

/////////////
//  STAGE  //
/////////////

// UDPStage is an egress stage that sends UDP datagrams.
type UDPStage[T msgSer] struct {
	*stage.Egress[T, udpWorker[T], *udpWorkerArgs, *udpWorker[T]]

	cfg *UDPConfig

	conn *net.UDPConn
}

// NewUDPStage returns a new UDP egress stage.
func NewUDPStage[T msgSer](inputConnector conn[T], cfg *UDPConfig) *UDPStage[T] {
	return &UDPStage[T]{
		Egress: stage.NewEgress[T, udpWorker[T], *udpWorkerArgs](
			"udp", inputConnector, cfg.PoolConfig,
		),

		cfg: cfg,
	}
}

// Init initializes the stage.
func (us *UDPStage[T]) Init(ctx context.Context) error {
	// Parse the IP address
	parsedAddr, err := netip.ParseAddr(us.cfg.IPAddr)
	if err != nil {
		return err
	}
	addr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(parsedAddr, us.cfg.Port))

	// Dial the UDP connection
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}

	us.conn = conn

	return us.Egress.Init(ctx, newUDPWorkerArgs(conn))
}
