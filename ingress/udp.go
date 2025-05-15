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

const (
	defaultUDPPayloadSize = 1474
)

type UDPData struct {
	Frame []byte
}

type UDPDataPool struct {
	pool *sync.Pool
}

var UDPDataPoolInstance = newUDPDataPool()

func newUDPDataPool() *UDPDataPool {
	return &UDPDataPool{
		pool: &sync.Pool{
			New: func() any {
				return &UDPData{
					Frame: make([]byte, defaultUDPPayloadSize),
				}
			},
		},
	}
}

func (p *UDPDataPool) Get() *UDPData {
	return p.pool.Get().(*UDPData)
}

func (p *UDPDataPool) Put(d *UDPData) {
	p.pool.Put(d)
}

type UDPConfig struct {
	*internal.WorkerPoolConfig

	IPAddr string
	Port   uint16
}

func NewDefaultUDPConfig() *UDPConfig {
	return &UDPConfig{
		WorkerPoolConfig: internal.NewDefaultWorkerPoolConfig(),

		IPAddr: "127.0.0.1",
		Port:   20_000,
	}
}

type UDP struct {
	l     *internal.Logger
	stats *internal.Stats

	cfg *UDPConfig

	addr *net.UDPAddr
	conn *net.UDPConn

	writerWg *sync.WaitGroup
	out      connector.Connector[*UDPData]

	workerPool *internal.WorkerPool[[]byte, *UDPData]
}

func NewUDP(cfg *UDPConfig) *UDP {
	l := internal.NewLogger("ingress", "udp")

	return &UDP{
		l:     l,
		stats: internal.NewStats(l),

		cfg: cfg,

		addr: net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(cfg.IPAddr), cfg.Port)),

		workerPool: internal.NewWorkerPool(l, newUDPWorkerGen(), cfg.WorkerPoolConfig),

		writerWg: &sync.WaitGroup{},
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

func (i *UDP) runWriter(ctx context.Context) {
	i.writerWg.Add(1)
	defer i.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case data := <-i.workerPool.OutputCh:
			if err := i.out.Write(data); err != nil {
				i.l.Warn("failed to write into output connector", "reason", err)
			}
		}
	}
}

func (i *UDP) Run(ctx context.Context) {
	i.l.Info("running")

	received := 0
	skipped := 0
	defer func() {
		i.l.Info("received frames", "count", received)
		i.l.Info("skipped frames", "count", skipped)
	}()

	go func() {
		<-ctx.Done()
		i.conn.Close()
	}()

	go i.stats.RunStats(ctx)

	go i.workerPool.Run(ctx)

	go i.runWriter(ctx)

	buf := make([]byte, defaultUDPPayloadSize)
	for {
		n, err := i.conn.Read(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				select {
				case <-ctx.Done():
				default:
					i.l.Error("failed to read connection", err)
				}

				return
			}

			i.l.Error("server failed to read", err)

			return
		}

		received++

		i.stats.IncrementItemCount()
		i.stats.IncrementByteCountBy(n)

		if n == 0 {
			i.l.Warn("received empty frame")
			continue
		}

		if !i.workerPool.AddTask(ctx, buf) {
			skipped++
		}
	}
}

func (i *UDP) Stop() {
	defer i.l.Info("stopped")

	i.out.Close()
	i.workerPool.Stop()
	i.writerWg.Wait()
}

func (i *UDP) SetOutput(connector connector.Connector[*UDPData]) {
	i.out = connector
}

type udpWorker struct {
	// *internal.BaseWorker
}

func newUDPWorkerGen() internal.WorkerGen[[]byte, *UDPData] {
	return func() internal.Worker[[]byte, *UDPData] {
		return &udpWorker{
			// BaseWorker: internal.NewBaseWorker(l),
		}
	}
}

func (w *udpWorker) DoWork(_ context.Context, buf []byte) (*UDPData, error) {
	// udpData := UDPDataPoolInstance.Get()

	udpData := &UDPData{
		Frame: make([]byte, len(buf)),
	}

	copy(udpData.Frame, buf)

	return udpData, nil
}
