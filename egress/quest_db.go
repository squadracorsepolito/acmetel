package egress

import (
	"context"
	"time"

	qdb "github.com/questdb/go-questdb-client/v3"
	"github.com/squadracorsepolito/acmetel/connector"
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
	l *internal.Logger

	cfg *QuestDBConfig

	senderPool *qdb.LineSenderPool

	in connector.Connector[*message.CANSignalBatch]

	workerPool *questDBWorkerPool
}

func NewQuestDB(cfg *QuestDBConfig) *QuestDB {
	tel := internal.NewTelemetry("egress", "quest_db")
	l := tel.Logger()

	return &QuestDB{
		l: l,

		cfg: cfg,

		workerPool: newQuestDBWorkerPool(tel, cfg.PoolConfig),
	}
}

func (e *QuestDB) Init(ctx context.Context) error {
	senderPool, err := qdb.PoolFromOptions(
		qdb.WithAddress(e.cfg.Address),
		qdb.WithHttp(),
		qdb.WithAutoFlushRows(128_000),
		qdb.WithRetryTimeout(time.Second),
	)
	if err != nil {
		return err
	}

	e.senderPool = senderPool

	if err := e.workerPool.Init(ctx, e.senderPool); err != nil {
		return err
	}

	return nil
}

func (e *QuestDB) Run(ctx context.Context) {
	e.l.Info("running")

	go e.workerPool.Run(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, err := e.in.Read()
		if err != nil {
			e.l.Warn("failed to read from input connector", "reason", err)
			continue
		}

		e.workerPool.AddTask(ctx, data)
	}
}

func (e *QuestDB) Stop() {
	defer e.l.Info("stopped")

	e.workerPool.Stop()

	if err := e.senderPool.Close(context.Background()); err != nil {
		e.l.Error("failed to close sender pool", err)
	}
}

func (e *QuestDB) SetInput(connector connector.Connector[*message.CANSignalBatch]) {
	e.in = connector
}

type questDBWorkerPool = worker.EgressPool[*message.CANSignalBatch, questDBWorker, *qdb.LineSenderPool, *questDBWorker]

func newQuestDBWorkerPool(tel *internal.Telemetry, cfg *worker.PoolConfig) *questDBWorkerPool {
	return worker.NewEgressPool[*message.CANSignalBatch, questDBWorker, *qdb.LineSenderPool](tel, cfg)
}

type questDBWorker struct {
	tel    *internal.Telemetry
	sender qdb.LineSender
}

func (w *questDBWorker) Init(ctx context.Context, senderPool *qdb.LineSenderPool) error {
	sender, err := senderPool.Sender(ctx)
	if err != nil {
		return err
	}

	w.sender = sender

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
