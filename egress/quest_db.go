package egress

import (
	"context"
	"time"

	qdb "github.com/questdb/go-questdb-client/v3"
	"github.com/squadracorsepolito/acmetel/connector"
	"github.com/squadracorsepolito/acmetel/internal"
)

type CANSignalBatch struct {
	Timestamp   time.Time
	SignalCount int
	Signals     []CANSignal
}

type CANSignalTable string

const (
	CANSignalTableFlag  CANSignalTable = "flag_signals"
	CANSignalTableInt   CANSignalTable = "int_signals"
	CANSignalTableFloat CANSignalTable = "float_signals"
	CANSignalTableEnum  CANSignalTable = "enum_signals"
)

type CANSignal struct {
	CANID      int64
	Name       string
	RawValue   int64
	Table      CANSignalTable
	ValueFlag  bool
	ValueInt   int64
	ValueFloat float64
	ValueEnum  string
	Unit       string
}

type QuestDBConfig struct {
	*internal.WorkerPoolConfig

	Address string
}

func NewDefaultQuestDBConfig() *QuestDBConfig {
	qdb.WithAutoFlushInterval(time.Second)

	return &QuestDBConfig{
		WorkerPoolConfig: internal.NewDefaultWorkerPoolConfig(),
		Address:          "localhost:9000",
	}
}

type QuestDB struct {
	l *internal.Logger

	cfg *QuestDBConfig

	senderPool *qdb.LineSenderPool

	in connector.Connector[*CANSignalBatch]

	workerPool *internal.WorkerPool[*CANSignalBatch, any]
}

func NewQuestDB(cfg *QuestDBConfig) *QuestDB {
	l := internal.NewLogger("egress", "quest_db")

	return &QuestDB{
		l: l,

		cfg: cfg,
	}
}

func (e *QuestDB) Init(_ context.Context) error {
	senderPool, err := qdb.PoolFromOptions(qdb.WithAddress(e.cfg.Address), qdb.WithHttp(), qdb.WithAutoFlushRows(100_000))
	if err != nil {
		return err
	}

	e.workerPool = internal.NewWorkerPool(e.l, newQuestDBWorkerGen(senderPool), e.cfg.WorkerPoolConfig)

	return nil
}

func (e *QuestDB) Run(ctx context.Context) {
	e.l.Info("running")

	go func() {
		<-ctx.Done()
		// e.sender.Close(context.Background())
	}()

	go e.workerPool.Run(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-e.workerPool.OutputCh:
			}
		}
	}()

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
}

func (e *QuestDB) SetInput(connector connector.Connector[*CANSignalBatch]) {
	e.in = connector
}

type questDBWorker struct {
	sender qdb.LineSender
}

func (w *questDBWorker) DoWork(ctx context.Context, data *CANSignalBatch) (any, error) {
	timestamp := data.Timestamp

	for i := range data.SignalCount {
		sig := data.Signals[i]

		var err error

		table := sig.Table
		switch table {
		case CANSignalTableFlag:
			err = w.sender.Table(string(table)).
				Symbol("name", sig.Name).
				Int64Column("can_id", sig.CANID).
				Int64Column("raw_value", sig.RawValue).
				BoolColumn("flag_value", sig.ValueFlag).
				At(ctx, timestamp)

		case CANSignalTableInt:
			err = w.sender.Table(string(table)).
				Symbol("name", sig.Name).
				Int64Column("can_id", sig.CANID).
				Int64Column("raw_value", sig.RawValue).
				Int64Column("integer_value", sig.ValueInt).
				StringColumn("unit", sig.Unit).
				At(ctx, timestamp)

		case CANSignalTableFloat:
			err = w.sender.Table(string(table)).
				Symbol("name", sig.Name).
				Int64Column("can_id", sig.CANID).
				Int64Column("raw_value", sig.RawValue).
				Float64Column("decimal_value", sig.ValueFloat).
				StringColumn("unit", sig.Unit).
				At(ctx, timestamp)

		case CANSignalTableEnum:
			err = w.sender.Table(string(table)).
				Symbol("name", sig.Name).
				Int64Column("can_id", sig.CANID).
				Int64Column("raw_value", sig.RawValue).
				StringColumn("enum_value", sig.ValueEnum).
				At(ctx, timestamp)
		}

		if err != nil {
			return nil, err
		}
	}

	return struct{}{}, nil
}

func newQuestDBWorkerGen(senderPool *qdb.LineSenderPool) internal.WorkerGen[*CANSignalBatch, any] {
	return func() internal.Worker[*CANSignalBatch, any] {
		sender, err := senderPool.Sender(context.Background())
		if err != nil {
			panic(err)
		}

		return &questDBWorker{
			sender: sender,
		}
	}
}
