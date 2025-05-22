package ingress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"

	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
	"go.opentelemetry.io/otel/attribute"
)

const (
	defaultUDPPayloadSize = 1474
)

type UDPConfig struct {
	*worker.PoolConfig

	IPAddr string
	Port   uint16
}

func NewDefaultUDPConfig() *UDPConfig {
	return &UDPConfig{
		PoolConfig: worker.DefaultPoolConfig(),

		IPAddr: "127.0.0.1",
		Port:   20_000,
	}
}

type UDP struct {
	tel *internal.Telemetry

	cfg *UDPConfig

	addr *net.UDPAddr
	conn *net.UDPConn

	writerWg *sync.WaitGroup
	out      connector.Connector[*message.UDPPayload]

	workerPool *udpWorkerPool
}

func NewUDP(cfg *UDPConfig) *UDP {
	tel := internal.NewTelemetry("ingress", "udp")

	return &UDP{
		tel: tel,

		cfg: cfg,

		addr: net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(cfg.IPAddr), cfg.Port)),

		workerPool: newUDPWorkerPool(tel, cfg.PoolConfig),

		writerWg: &sync.WaitGroup{},
	}
}

func (i *UDP) Init(ctx context.Context) error {
	conn, err := net.ListenUDP("udp", i.addr)
	if err != nil {
		return err
	}

	i.conn = conn

	i.workerPool.Init(ctx, conn)

	return nil
}

func (i *UDP) runWriter(ctx context.Context) {
	i.writerWg.Add(1)
	defer i.writerWg.Done()

	for {
		select {
		case <-ctx.Done():
			return

		case data := <-i.workerPool.GetOutputCh():
			if err := i.out.Write(data); err != nil {
				i.tel.LogWarn("failed to write into output connector", "reason", err)
			}
		}
	}
}

func (i *UDP) Run(ctx context.Context) {
	i.tel.LogInfo("running")

	received := 0
	skipped := 0
	defer func() {
		i.tel.LogInfo("received frames", "count", received)
		i.tel.LogInfo("skipped frames", "count", skipped)
	}()

	go i.runWriter(ctx)

	i.workerPool.Run(ctx)
}

func (i *UDP) Stop() {
	defer i.tel.LogInfo("stopped")

	i.out.Close()
	i.workerPool.Stop()
	i.writerWg.Wait()
}

func (i *UDP) SetOutput(connector connector.Connector[*message.UDPPayload]) {
	i.out = connector
}

type udpWorkerPool = worker.IngressPool[udpWorker, *net.UDPConn, *message.UDPPayload, *udpWorker]

type udpWorker struct {
	tel *internal.Telemetry

	conn *net.UDPConn
	buf  []byte
}

func newUDPWorkerPool(tel *internal.Telemetry, cfg *worker.PoolConfig) *udpWorkerPool {
	return worker.NewIngressPool[udpWorker, *net.UDPConn, *message.UDPPayload](tel, cfg)
}

func (w *udpWorker) Init(ctx context.Context, conn *net.UDPConn) error {
	w.conn = conn

	go func() {
		<-ctx.Done()
		w.conn.Close()
	}()

	w.buf = make([]byte, defaultUDPPayloadSize)
	return nil
}

func (w *udpWorker) Receive(ctx context.Context) (*message.UDPPayload, bool, error) {
	_, err := w.conn.Read(w.buf)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			select {
			case <-ctx.Done():
				return nil, true, nil

			default:
				w.tel.LogError("failed to read connection", err)
			}

			return nil, true, err
		}

		w.tel.LogError("server failed to read", err)

		return nil, false, err
	}

	_, span := w.tel.NewTrace(ctx, "process UDP frame")
	defer span.End()

	dataLen := len(w.buf)
	data := make([]byte, dataLen)
	copy(data, w.buf)

	payload := message.NewUDPPayload(data, dataLen)

	span.SetAttributes(attribute.Int("data_len", dataLen))
	payload.SaveSpan(span)

	return payload, false, nil
}

func (w *udpWorker) Stop(_ context.Context) error {
	return nil
}

func (w *udpWorker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}
