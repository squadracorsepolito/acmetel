package questdb

import (
	"context"
	"math/big"
	"sync/atomic"
	"time"

	qdb "github.com/questdb/go-questdb-client/v3"
	"github.com/squadracorsepolito/acmetel/internal"
)

type workerArgs[T internal.Message] struct {
	handler    Handler[T]
	senderPool *qdb.LineSenderPool
}

type worker[T internal.Message] struct {
	tel *internal.Telemetry

	handler Handler[T]
	sender  qdb.LineSender

	// Telemetry metrics
	insertedRows atomic.Int64
}

func (w *worker[T]) SetTelemetry(tel *internal.Telemetry) {
	w.tel = tel
}

func (w *worker[T]) initMetrics() {
	w.tel.NewCounter("inserted_rows", func() int64 { return w.insertedRows.Load() })
}

func (w *worker[T]) Init(ctx context.Context, args *workerArgs[T]) error {
	// Set the handler
	w.handler = args.handler

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

func (w *worker[T]) Deliver(ctx context.Context, msg T) error {
	// Add span to trace the funtion
	ctx, span := w.tel.NewTrace(ctx, "insert message")
	defer span.End()

	// Get the result from the user defined handler
	res, err := w.handler.Handle(ctx, msg)
	if err != nil {
		return err
	}
	span.AddEvent("message handled")

	// Check if the result is nil
	if res == nil {
		w.tel.LogWarn("received nil result from handler")
		return nil
	}

	tmpInsRows := int64(0)

	// For each row, build the corresponding questDB query
	for row := range res.iterRows() {
		query := w.sender.Table(row.Table)

		for _, col := range row.Columns {
			switch col.Type {
			case ColumnTypeBool:
				query.BoolColumn(col.Name, col.Value.(bool))
			case ColumnTypeInt:
				query.Int64Column(col.Name, col.Value.(int64))
			case ColumnTypeLong:
				query.Long256Column(col.Name, col.Value.(*big.Int))
			case ColumnTypeFloat:
				query.Float64Column(col.Name, col.Value.(float64))
			case ColumnTypeSymbol:
				query.Symbol(col.Name, col.Value.(string))
			case ColumnTypeString:
				query.StringColumn(col.Name, col.Value.(string))
			case ColumnTypeTimestamp:
				query.TimestampColumn(col.Name, col.Value.(time.Time))
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

func (w *worker[T]) Stop(ctx context.Context) error {
	// Close the sender
	select {
	case <-ctx.Done():
		return w.sender.Close(context.Background())
	default:
		return w.sender.Close(ctx)
	}
}
