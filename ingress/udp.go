package ingress

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

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
	*stage[*message.UDPPayload, *UDPConfig, udpWorker, *net.UDPConn, *udpWorker]

	undConn *net.UDPConn
}

func NewUDP(cfg *UDPConfig) *UDP {
	return &UDP{
		stage: newStage[*message.UDPPayload, *UDPConfig, udpWorker, *net.UDPConn]("udp", cfg),
	}
}

func (i *UDP) Init(ctx context.Context) error {
	addr := net.UDPAddrFromAddrPort(netip.AddrPortFrom(netip.MustParseAddr(i.cfg.IPAddr), i.cfg.Port))
	udpConn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}

	i.undConn = udpConn

	return i.init(ctx, udpConn)
}

func (i *UDP) Run(ctx context.Context) {
	i.run(ctx)
}

func (i *UDP) Stop() {
	i.close()
}

type udpWorker struct {
	tel *internal.Telemetry

	udpConn *net.UDPConn
	buf     []byte

	receivedBytes atomic.Int64
}

func (w *udpWorker) Init(ctx context.Context, conn *net.UDPConn) error {
	w.udpConn = conn

	go func() {
		<-ctx.Done()
		w.udpConn.Close()
	}()

	w.buf = make([]byte, defaultUDPPayloadSize)

	w.tel.NewCounter("received_bytes", func() int64 { return w.receivedBytes.Load() })

	return nil
}

func (w *udpWorker) Receive(ctx context.Context) (*message.UDPPayload, bool, error) {
	_, err := w.udpConn.Read(w.buf)
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

	payload.SetReceiveTime(time.Now())

	w.receivedBytes.Add(int64(dataLen))

	return payload, false, nil
}

func (w *udpWorker) Stop(_ context.Context) error {
	return nil
}

func (w *udpWorker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}
