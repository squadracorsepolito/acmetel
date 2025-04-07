package ingress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
)

type UDPData struct {
	Frame []byte
}

type UDPConfig struct {
	IPAddr     string
	Port       uint16
	BufferSize int
	WorkerNum  int
}

func NewDefaultUDPConfig() *UDPConfig {
	return &UDPConfig{
		IPAddr:     "127.0.0.1",
		Port:       20_000,
		BufferSize: 2048,
		WorkerNum:  1,
	}
}

type UDP struct {
	l     *internal.Logger
	stats *internal.Stats

	cfg *UDPConfig

	addr *net.UDPAddr
	conn *net.UDPConn

	pool *sync.Pool
	out  connector.Connector[*UDPData]

	workerNum int
	workerWg  *sync.WaitGroup
	workerCh  chan []byte
}

func NewUDP(cfg *UDPConfig) *UDP {
	l := internal.NewLogger("ingress", "udp")

	return &UDP{
		l:     l,
		stats: internal.NewStats(l),

		cfg: cfg,

		addr: net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(cfg.IPAddr), cfg.Port)),

		pool: &sync.Pool{
			New: func() any {
				return &UDPData{
					Frame: make([]byte, cfg.BufferSize),
				}
			},
		},

		workerNum: cfg.WorkerNum,
		workerWg:  &sync.WaitGroup{},
		workerCh:  make(chan []byte, cfg.WorkerNum*16),
	}
}

func (i *UDP) Init(ctx context.Context) error {
	conn, err := net.ListenUDP("udp", i.addr)
	if err != nil {
		return err
	}

	i.conn = conn

	return nil
}

func (i *UDP) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case buf := <-i.workerCh:
			data := i.pool.Get().(*UDPData)
			data.Frame = buf

			if err := i.out.Write(data); err != nil {
				i.l.Warn("failed to write into output connector", "reason", err)
			}

			i.pool.Put(data)
		}
	}
}

func (i *UDP) Run(ctx context.Context) {
	go func() {
		<-ctx.Done()
		i.conn.Close()
	}()

	buf := make([]byte, i.cfg.BufferSize)

	i.l.Info("starting run")
	defer i.l.Info("quitting run")

	go i.stats.RunStats(ctx)

	received := 0
	skipped := 0
	defer func() {
		i.l.Info("received frames", "count", received)
		i.l.Info("skipped frames", "count", skipped)
	}()

	i.workerWg.Add(i.workerNum)

	for range i.workerNum {
		go func() {
			i.runWorker(ctx)
			i.workerWg.Done()
		}()
	}

	for {
		n, err := i.conn.Read(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				select {
				case <-ctx.Done():
				default:
					i.l.Error("failed to read connection", "reason", err)
				}

				return
			}

			i.l.Error("server failed to read", "reason", err)

			return
		}

		received++

		i.stats.IncrementItemCount()
		i.stats.IncrementByteCountBy(n)

		if n == 0 {
			i.l.Warn("received empty frame")
			continue
		}

		select {
		case i.workerCh <- buf:
		default:
			skipped++
		}
	}
}

func (i *UDP) Stop() {
	i.out.Close()

	i.workerWg.Wait()
	close(i.workerCh)
}

func (i *UDP) SetOutput(connector connector.Connector[*UDPData]) {
	i.out = connector
}
