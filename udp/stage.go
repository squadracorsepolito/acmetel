package udp

import (
	"context"
	"net"
	"net/netip"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal/stage"
)

type Stage struct {
	*stage.Ingress[*Message, worker, *workerArgs, *worker]

	ipAddr string
	port   uint16
}

func NewStage(outputConnector connector.Connector[*Message], cfg *Config) *Stage {
	return &Stage{
		Ingress: stage.NewIngress[*Message, worker, *workerArgs](
			"udp", outputConnector, cfg.PoolConfig,
		),

		ipAddr: cfg.IPAddr,
		port:   cfg.Port,
	}
}

func (s *Stage) Init(ctx context.Context) error {
	parsedAddr, err := netip.ParseAddr(s.ipAddr)
	if err != nil {
		return err
	}

	addr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(parsedAddr, s.port))
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	return s.Ingress.Init(ctx, &workerArgs{conn: conn})
}

func (s *Stage) Stop() {
	s.Ingress.Close()
}
