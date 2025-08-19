package questdb

import (
	"iter"
	"math/big"
	"slices"
	"time"

	"github.com/squadracorsepolito/acmetel/internal"
)

type Message struct {
	internal.BaseMessage

	Timestamp time.Time
	Rows      []*Row
}

func NewMessage() *Message {
	return &Message{}
}

func (m *Message) AddRow(row *Row) {
	if row != nil {
		m.Rows = append(m.Rows, row)
	}
}

func (m *Message) AddRows(rows ...*Row) {
	if rows == nil {
		return
	}

	if len(m.Rows) == 0 {
		m.Rows = rows
		return
	}

	m.Rows = append(m.Rows, rows...)
}

func (m *Message) iterRows() iter.Seq[*Row] {
	return slices.Values(m.Rows)
}

type ColumnType int

const (
	ColumnTypeBool ColumnType = iota
	ColumnTypeInt
	ColumnTypeLong
	ColumnTypeFloat
	ColumnTypeString
	ColumnTypeTimestamp
)

type Column struct {
	Name  string
	Type  ColumnType
	Value any
}

func newColumn(name string, typ ColumnType, value any) *Column {
	return &Column{
		Name:  name,
		Type:  typ,
		Value: value,
	}
}

func NewBoolColumn(name string, value bool) *Column {
	return newColumn(name, ColumnTypeBool, value)
}

func NewIntColumn(name string, value int64) *Column {
	return newColumn(name, ColumnTypeInt, value)
}

func NewLongColumn(name string, value *big.Int) *Column {
	return newColumn(name, ColumnTypeLong, value)
}

func NewFloatColumn(name string, value float64) *Column {
	return newColumn(name, ColumnTypeFloat, value)
}

func NewStringColumn(name string, value string) *Column {
	return newColumn(name, ColumnTypeString, value)
}

func NewTimestampColumn(name string, value time.Time) *Column {
	return newColumn(name, ColumnTypeTimestamp, value)
}

type Symbol struct {
	Name  string
	Value string
}

func NewSymbol(name string, value string) *Symbol {
	return &Symbol{
		Name:  name,
		Value: value,
	}
}

type Row struct {
	Table   string
	Symbols []*Symbol
	Columns []*Column
}

func NewRow(table string) *Row {
	return &Row{
		Table: table,
	}
}

func (r *Row) AddSymbol(symbol *Symbol) {
	if symbol != nil {
		r.Symbols = append(r.Symbols, symbol)
	}
}

func (r *Row) AddColumn(column *Column) {
	if column != nil {
		r.Columns = append(r.Columns, column)
	}
}
