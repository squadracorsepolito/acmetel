package acmetel

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"

	"github.com/squadracorsepolito/acmetel/internal"
)

type ServerUDP struct {
	l *slog.Logger

	cfg  *ServerUDPConfig
	rb   *internal.RingBuffer[[]byte]
	addr *net.UDPAddr
}

type ServerUDPConfig struct {
	IPAddr     string
	Port       uint16
	BufferSize int
}

func NewDefaultServerUDPConfig() *ServerUDPConfig {
	return &ServerUDPConfig{
		IPAddr:     "127.0.0.1",
		Port:       20_000,
		BufferSize: 2048,
	}
}

func NewServerUDP(rb *internal.RingBuffer[[]byte], cfg *ServerUDPConfig) *ServerUDP {
	return &ServerUDP{
		l: slog.Default(),

		cfg:  cfg,
		rb:   rb,
		addr: net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(cfg.IPAddr), cfg.Port)),
	}
}

func (s *ServerUDP) Run(ctx context.Context) error {
	conn, err := net.ListenUDP("udp", s.addr)
	if err != nil {
		return err
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			default:
			}
		}
	}()

	buf := make([]byte, s.cfg.BufferSize)

	for {
		select {
		case <-ctx.Done():
			s.l.Info("server stopped", "reason", ctx.Err())
			return nil
		default:
		}

		_, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				select {
				case <-ctx.Done():
					s.l.Info("server stopped", "reason", ctx.Err())

				default:
					s.l.Error("server conn closed")
				}

				return nil
			}

			return err
		}

		if err := s.rb.Put(ctx, buf); err != nil {
			s.l.Error("server failed to put to ring buffer", "reason", err)
		}
	}
}
