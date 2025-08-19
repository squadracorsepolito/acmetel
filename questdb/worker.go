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
		query := w.sender.Table(row.Table)

		for _, symbol := range row.Symbols {
			query.Symbol(symbol.Name, symbol.Value)
		}

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

	// // Get the result from the user defined handler
	// res, err := w.handler.Handle(ctx, msg)
	// if err != nil {
	// 	return err
	// }
	// span.AddEvent("message handled")

	// // Check if the result is nil
	// if res == nil {
	// 	w.tel.LogWarn("received nil result from handler")
	// 	return nil
	// }

	// tmpInsRows := int64(0)

	// // For each row, build the corresponding questDB query
	// for row := range res.iterRows() {
	// 	query := w.sender.Table(row.Table)

	// 	for _, col := range row.Columns {
	// 		switch col.Type {
	// 		case ColumnTypeBool:
	// 			query.BoolColumn(col.Name, col.Value.(bool))
	// 		case ColumnTypeInt:
	// 			query.Int64Column(col.Name, col.Value.(int64))
	// 		case ColumnTypeLong:
	// 			query.Long256Column(col.Name, col.Value.(*big.Int))
	// 		case ColumnTypeFloat:
	// 			query.Float64Column(col.Name, col.Value.(float64))
	// 		case ColumnTypeSymbol:
	// 			query.Symbol(col.Name, col.Value.(string))
	// 		case ColumnTypeString:
	// 			query.StringColumn(col.Name, col.Value.(string))
	// 		case ColumnTypeTimestamp:
	// 			query.TimestampColumn(col.Name, col.Value.(time.Time))
	// 		}
	// 	}

	// 	if err := query.At(ctx, msg.GetTimestamp()); err != nil {
	// 		return err
	// 	}

	// 	tmpInsRows++
	// }

	// w.insertedRows.Add(tmpInsRows)

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
