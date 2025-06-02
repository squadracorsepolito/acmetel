package egress

import (
	"context"
	"fmt"
	"strconv"

	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
	"github.com/valkey-io/valkey-go"
)

type ValkeyConfig struct {
	*worker.PoolConfig

	Addr string
}

func NewDefaultValkeyConfig() *ValkeyConfig {
	return &ValkeyConfig{
		PoolConfig: worker.DefaultPoolConfig(),

		Addr: "localhost:6379",
	}
}

type Valkey struct {
	*stage[*message.CANSignalBatch, *ValkeyConfig, valkeyWorker, valkey.Client, *valkeyWorker]
}

func NewValkey(cfg *ValkeyConfig) *Valkey {
	return &Valkey{
		stage: newStage[*message.CANSignalBatch, *ValkeyConfig, valkeyWorker, valkey.Client, *valkeyWorker]("valkey", cfg),
	}
}

func (e *Valkey) Init(ctx context.Context) error {
	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{e.cfg.Addr},
	})
	if err != nil {
		return err
	}

	return e.init(ctx, client)
}

func (e *Valkey) Run(ctx context.Context) {
	e.run(ctx)
}

func (e *Valkey) Stop() {
	e.close()
}

type valkeyWorker struct {
	tel *internal.Telemetry

	client valkey.Client

	commands  valkey.Commands
	batchSize int
}

func (w *valkeyWorker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *valkeyWorker) Init(_ context.Context, client valkey.Client) error {
	w.client = client

	w.batchSize = 5_000

	w.commands = make(valkey.Commands, 0, w.batchSize)

	return nil
}

func (w *valkeyWorker) Deliver(ctx context.Context, data *message.CANSignalBatch) error {
	defer message.PutCANSignalBatch(data)

	timestamp := data.Timestamp.UnixNano()

	for i := range data.SignalCount {
		sig := data.Signals[i]

		key := fmt.Sprintf("%s_%d", sig.Name, timestamp)
		value := strconv.Itoa(int(sig.ValueInt))

		cmd := w.client.B().Set().Key(key).Value(value).Build()
		w.commands = append(w.commands, cmd)
	}

	if len(w.commands) >= w.batchSize {
		for _, resp := range w.client.DoMulti(ctx, w.commands...) {
			if err := resp.Error(); err != nil {
				return err
			}
		}

		w.commands = make(valkey.Commands, 0, w.batchSize)
	}

	return nil
}

func (w *valkeyWorker) Stop(ctx context.Context) error {
	w.client.Close()
	return nil
}
