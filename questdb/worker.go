package questdb

import (
	"context"
	"math/big"
	"sync/atomic"
	"time"

	qdb "github.com/questdb/go-questdb-client/v3"
	"github.com/squadracorsepolito/acmetel/internal"
)

type workerArgs struct {
	senderPool *qdb.LineSenderPool
}

func newWorkerArgs(senderPool *qdb.LineSenderPool) *workerArgs {
	return &workerArgs{senderPool: senderPool}
}

type worker struct {
	tel *internal.Telemetry

	sender qdb.LineSender

	// Telemetry metrics
	insertedRows atomic.Int64
}

func (w *worker) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *worker) initMetrics() {
	w.tel.NewCounter("inserted_rows", func() int64 { return w.insertedRows.Load() })
}

func (w *worker) Init(ctx context.Context, args *workerArgs) error {
	// Get and set the sender from the pool
	sender, err := args.senderPool.Sender(ctx)
	if err != nil {
		return err
	}
	w.sender = sender

	// Initialize the metrics
	w.initMetrics()

	return nil
}

func (w *worker) Deliver(ctx context.Context, msg *Message) error {
	// Add span to trace the funtion
	ctx, span := w.tel.NewTrace(ctx, "insert message")
	defer span.End()

	tmpInsRows := int64(0)

	for row := range msg.iterRows() {
		query := w.sender.Table(row.table)

		for _, symbol := range row.symbols {
			query.Symbol(symbol.name, symbol.value)
		}

		for _, col := range row.columns {
			switch col.typ {
			case ColumnTypeBool:
				query.BoolColumn(col.name, col.value.(bool))
			case ColumnTypeInt:
				query.Int64Column(col.name, col.value.(int64))
			case ColumnTypeLong:
				query.Long256Column(col.name, col.value.(*big.Int))
			case ColumnTypeFloat:
				query.Float64Column(col.name, col.value.(float64))
			case ColumnTypeString:
				query.StringColumn(col.name, col.value.(string))
			case ColumnTypeTimestamp:
				query.TimestampColumn(col.name, col.value.(time.Time))
			}
		}

		if err := query.At(ctx, msg.GetTimestamp()); err != nil {
			return err
		}

		tmpInsRows++
	}

	w.insertedRows.Add(tmpInsRows)

	return nil
}

func (w *worker) Close(ctx context.Context) error {
	// Close the sender
	select {
	case <-ctx.Done():
		return w.sender.Close(context.Background())
	default:
		return w.sender.Close(ctx)
	}
}
