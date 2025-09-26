package egress

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"

	"github.com/squadracorsepolito/acmetel/internal/message"
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

///////////////
//  MESSAGE  //
///////////////

// UDPMessage represents a UDP message to be sent.
type UDPMessage struct {
	message.Base

	// Payload is the bytes of the UDP payload.
	Payload []byte
	// PayloadSize is the size of the UDP payload.
	PayloadSize int
}

// NewUDPMessage returns a new UDP message with the given payload.
// It sets the payload size to the length of the provided payload.
func NewUDPMessage(payload []byte) *UDPMessage {
	return &UDPMessage{
		Payload:     payload,
		PayloadSize: len(payload),
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

type udpWorker struct {
	pool.BaseWorker

	conn *net.UDPConn

	// Metrics
	deliveredBytes atomic.Int64
}

func (uw *udpWorker) initMetrics() {
	uw.Tel.NewCounter("delivered_bytes", func() int64 { return uw.deliveredBytes.Load() })
}

func (uw *udpWorker) Init(_ context.Context, args *udpWorkerArgs) error {
	uw.conn = args.conn

	uw.initMetrics()

	return nil
}

func (uw *udpWorker) Deliver(ctx context.Context, udpMsg *UDPMessage) error {
	// Extract the span context from the input message
	_, span := uw.Tel.NewTrace(udpMsg.LoadSpanContext(ctx), "deliver UDP message")
	defer span.End()

	_, err := uw.conn.Write(udpMsg.Payload)
	if err != nil {
		return err
	}

	span.SetAttributes(attribute.Int("payload_size", udpMsg.PayloadSize))

	// Update metrics
	uw.deliveredBytes.Add(int64(udpMsg.PayloadSize))

	return nil
}

func (uw *udpWorker) Close(_ context.Context) error {
	return nil
}

/////////////
//  STAGE  //
/////////////

// UDPStage is an egress stage that sends UDP datagrams.
type UDPStage struct {
	*stage.Egress[*UDPMessage, udpWorker, *udpWorkerArgs, *udpWorker]

	cfg *UDPConfig

	conn *net.UDPConn
}

// NewUDPStage returns a new UDP egress stage.
func NewUDPStage(inputConnector conn[*UDPMessage], cfg *UDPConfig) *UDPStage {
	return &UDPStage{
		Egress: stage.NewEgress[*UDPMessage, udpWorker, *udpWorkerArgs](
			"udp", inputConnector, cfg.PoolConfig,
		),

		cfg: cfg,
	}
}

// Init initializes the stage.
func (us *UDPStage) Init(ctx context.Context) error {
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
