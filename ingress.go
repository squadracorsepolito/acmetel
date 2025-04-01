package acmetel

import (
	"context"
	"errors"
	"net"
	"net/netip"

	"github.com/squadracorsepolito/acmetel/internal"
)

type UDPIngress struct {
	l *logger

	cfg *UDPIngressConfig

	addr *net.UDPAddr
	conn *net.UDPConn

	out *internal.RingBuffer[[]byte]
}

type UDPIngressConfig struct {
	IPAddr     string
	Port       uint16
	BufferSize int
}

func NewDefaultUDPIngressConfig() *UDPIngressConfig {
	return &UDPIngressConfig{
		IPAddr:     "127.0.0.1",
		Port:       20_000,
		BufferSize: 2048,
	}
}

func NewUDPIngress(cfg *UDPIngressConfig) *UDPIngress {
	return &UDPIngress{
		l: newLogger(stageKindIngress, "ingress-udp"),

		cfg: cfg,

		addr: net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(cfg.IPAddr), cfg.Port)),
	}
}

func (in *UDPIngress) Init(ctx context.Context) error {
	if in.out == nil {
		return errors.New("output connector not set")
	}

	in.out.Init(ctx)

	conn, err := net.ListenUDP("udp", in.addr)
	if err != nil {
		return err
	}

	in.conn = conn

	return nil
}

func (in *UDPIngress) Run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		in.conn.Close()
	}()

	buf := make([]byte, in.cfg.BufferSize)

	in.l.Info("starting run")
	defer in.l.Info("quitting run")

	for {
		n, err := in.conn.Read(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				select {
				case <-ctx.Done():
				default:
					in.l.Error("failed to read connection", "reason", err)
				}

				return
			}

			in.l.Error("server failed to read", "reason", err)

			return
		}

		if n == 0 {
			in.l.Warn("received empty frame")
			continue
		}

		if err := in.out.Write(ctx, buf); err != nil {
			in.l.Warn("failed to write into output connector", "reason", err)
		}
	}
}

func (in *UDPIngress) Stop() {}

func (in *UDPIngress) SetOutput(connector *internal.RingBuffer[[]byte]) {
	in.out = connector
}
