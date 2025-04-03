package acmetel

import (
	"context"
	"errors"
	"net"
	"net/netip"
)

type UDPIngress struct {
	*stats

	l *logger

	cfg *UDPIngressConfig

	addr *net.UDPAddr
	conn *net.UDPConn

	out Connector[[2048]byte]
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
	l := newLogger(stageKindIngress, "ingress-udp")

	return &UDPIngress{
		stats: newStats(l),

		l: l,

		cfg: cfg,

		addr: net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(cfg.IPAddr), cfg.Port)),
	}
}

func (in *UDPIngress) Init(ctx context.Context) error {
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

	buf := [2048]byte{} //make([]byte, in.cfg.BufferSize)

	in.l.Info("starting run")
	defer in.l.Info("quitting run")

	go in.stats.runStats(ctx)

	count := 0
	defer func() {
		in.l.Info("processed frames", "count", count)
	}()

	// tmp := 8
	// ch := make(chan []byte, tmp)

	// // rb := NewRingBuffer[[]byte](uint64(tmp * 1000))

	// for i := 0; i < tmp; i++ {
	// 	go func() {
	// 		for {
	// 			select {
	// 			case <-ctx.Done():
	// 				return
	// 			case buf := <-ch:
	// 				if err := in.out.Write(buf); err != nil {
	// 					in.l.Warn("failed to write into output connector", "reason", err)
	// 				}
	// 				// default:
	// 			}

	// 			// buf, err := rb.Read()
	// 			// if err != nil {
	// 			// 	return
	// 			// }

	// 			// if err := in.out.Write(buf); err != nil {
	// 			// 	in.l.Warn("failed to write into output connector", "reason", err)
	// 			// }
	// 		}
	// 	}()
	// }

	for {
		n, err := in.conn.Read(buf[:])
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

		count++

		in.incrementItemCount()
		in.incrementByteCountBy(n)

		if n == 0 {
			in.l.Warn("received empty frame")
			continue
		}

		if err := in.out.Write(buf); err != nil {
			in.l.Warn("failed to write into output connector", "reason", err)
		}

		// ch <- buf
		// rb.Write(buf)
	}

}

func (in *UDPIngress) Stop() {
	in.out.Close()
}

func (in *UDPIngress) SetOutput(connector Connector[[2048]byte]) {
	in.out = connector
}
