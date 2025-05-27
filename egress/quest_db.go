package egress

import (
	"context"
	"sync/atomic"
	"time"

	qdb "github.com/questdb/go-questdb-client/v3"
	"github.com/squadracorsepolito/acmetel/internal"
	"github.com/squadracorsepolito/acmetel/message"
	"github.com/squadracorsepolito/acmetel/worker"
	"go.opentelemetry.io/otel/attribute"
)

type QuestDBConfig struct {
	*worker.PoolConfig

	Address string
}

func NewDefaultQuestDBConfig() *QuestDBConfig {
	qdb.WithAutoFlushInterval(time.Second)

	return &QuestDBConfig{
		PoolConfig: worker.DefaultPoolConfig(),
		Address:    "localhost:9000",
	}
}

type QuestDB struct {
	*stage[*message.CANSignalBatch, *QuestDBConfig, questDBWorker, *qdb.LineSenderPool, *questDBWorker]

	senderPool *qdb.LineSenderPool
}

func NewQuestDB(cfg *QuestDBConfig) *QuestDB {
	return &QuestDB{
		stage: newStage[*message.CANSignalBatch, *QuestDBConfig, questDBWorker, *qdb.LineSenderPool]("quest_db", cfg),
	}
}

func (e *QuestDB) Init(ctx context.Context) error {
	senderPool, err := qdb.PoolFromOptions(
		qdb.WithAddress(e.cfg.Address),
		qdb.WithHttp(),
		qdb.WithAutoFlushRows(75_000),
		qdb.WithRetryTimeout(time.Second),
	)
	if err != nil {
		return err
	}

	e.senderPool = senderPool

	return e.init(ctx, senderPool)
}

func (e *QuestDB) Run(ctx context.Context) {
	e.run(ctx)
}

func (e *QuestDB) Stop() {
	e.close()

	if err := e.senderPool.Close(context.Background()); err != nil {
		e.tel.LogError("failed to close sender pool", err)
	}
}

type questDBWorker struct {
	tel *internal.Telemetry

	sender qdb.LineSender

	deliveredRows atomic.Int64
}

func (w *questDBWorker) Init(ctx context.Context, senderPool *qdb.LineSenderPool) error {
	sender, err := senderPool.Sender(ctx)
	if err != nil {
		return err
	}

	w.sender = sender

	w.tel.NewCounter("delivered_rows", func() int64 { return w.deliveredRows.Load() })

	return nil
}

func (w *questDBWorker) Deliver(ctx context.Context, data *message.CANSignalBatch) error {
	defer message.PutCANSignalBatch(data)

	ctx, span := w.tel.NewTrace(ctx, "insert CAN signals")
	defer span.End()

	span.SetAttributes(attribute.Int("signal_count", data.SignalCount))

	timestamp := data.Timestamp

	for i := range data.SignalCount {
		sig := data.Signals[i]

		var err error

		table := sig.Table
		switch table {
		case message.CANSignalTableFlag:
			err = w.sender.Table(table.String()).
				Symbol("name", sig.Name).
				Int64Column("can_id", sig.CANID).
				Int64Column("raw_value", sig.RawValue).
				BoolColumn("flag_value", sig.ValueFlag).
				At(ctx, timestamp)

		case message.CANSignalTableInt:
			err = w.sender.Table(table.String()).
				Symbol("name", sig.Name).
				Int64Column("can_id", sig.CANID).
				Int64Column("raw_value", sig.RawValue).
				Int64Column("integer_value", sig.ValueInt).
				At(ctx, timestamp)

		case message.CANSignalTableFloat:
			err = w.sender.Table(table.String()).
				Symbol("name", sig.Name).
				Int64Column("can_id", sig.CANID).
				Int64Column("raw_value", sig.RawValue).
				Float64Column("decimal_value", sig.ValueFloat).
				At(ctx, timestamp)

		case message.CANSignalTableEnum:
			err = w.sender.Table(table.String()).
				Symbol("name", sig.Name).
				Int64Column("can_id", sig.CANID).
				Int64Column("raw_value", sig.RawValue).
				StringColumn("enum_value", sig.ValueEnum).
				At(ctx, timestamp)
		}

		if err != nil {
			return err
		}
	}

	w.deliveredRows.Add(int64(data.SignalCount))

	return nil
}

func (w *questDBWorker) Stop(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return w.sender.Close(context.Background())
	default:
		return w.sender.Close(ctx)
	}
}

func (w *questDBWorker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}
