package questdb

import (
	"context"
	"iter"
	"math/big"
	"slices"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
)

type Handler[T internal.Message] interface {
	Init(ctx context.Context) error
	Handle(ctx context.Context, msg T) (*Result, error)
	Close()
}

type ColumnType int

const (
	ColumnTypeBool ColumnType = iota
	ColumnTypeInt
	ColumnTypeLong
	ColumnTypeFloat
	ColumnTypeSymbol
	ColumnTypeString
	ColumnTypeTimestamp
)

type Column struct {
	Name  string
	Type  ColumnType
	Value any
}

func NewColumn(name string, typ ColumnType, value any) *Column {
	return &Column{
		Name:  name,
		Type:  typ,
		Value: value,
	}
}

func NewBoolColumn(name string, value bool) *Column {
	return NewColumn(name, ColumnTypeBool, value)
}

func NewIntColumn(name string, value int64) *Column {
	return NewColumn(name, ColumnTypeInt, value)
}

func NewLongColumn(name string, value *big.Int) *Column {
	return NewColumn(name, ColumnTypeLong, value)
}

func NewFloatColumn(name string, value float64) *Column {
	return NewColumn(name, ColumnTypeFloat, value)
}

func NewSymbolColumn(name string, value string) *Column {
	return NewColumn(name, ColumnTypeSymbol, value)
}

func NewStringColumn(name string, value string) *Column {
	return NewColumn(name, ColumnTypeString, value)
}

func NewTimestampColumn(name string, value time.Time) *Column {
	return NewColumn(name, ColumnTypeTimestamp, value)
}

type Row struct {
	Table   string
	Columns []*Column
}

func NewRow(table string, columns ...*Column) *Row {
	return &Row{
		Table:   table,
		Columns: columns,
	}
}

func (r *Row) AddColumn(column *Column) {
	if column != nil {
		r.Columns = append(r.Columns, column)
	}
}

type Result struct {
	Rows []*Row
}

func NewResult(rows ...*Row) *Result {
	return &Result{
		Rows: rows,
	}
}

func (r *Result) AddRow(row *Row) {
	if row != nil {
		r.Rows = append(r.Rows, row)
	}
}

func (r *Result) iterRows() iter.Seq[*Row] {
	return slices.Values(r.Rows)
}
