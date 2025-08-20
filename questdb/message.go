package questdb

import (
	"iter"
	"math/big"
	"slices"
	"time"

	"github.com/squadracorsepolito/acmetel/internal/message"
)

// Message represents a QuestDB message.
// It contains the definition of the rows and columns to be inserted
// into the database.
type Message struct {
	message.Base

	rows []*Row
}

// NewMessage returns a new QuestDB message.
func NewMessage() *Message {
	return &Message{}
}

// AddRow adds a row to the message.
func (m *Message) AddRow(row *Row) {
	m.rows = append(m.rows, row)
}

// AddRows adds multiple rows to the message.
func (m *Message) AddRows(rows ...*Row) {
	if len(m.rows) == 0 {
		m.rows = rows
		return
	}

	m.rows = append(m.rows, rows...)
}

// GetRows returns the rows of the message.
func (m *Message) GetRows() []*Row {
	return m.rows
}

func (m *Message) iterRows() iter.Seq[*Row] {
	return slices.Values(m.rows)
}

// ColumnType represents the type of a column.
// It does not include the symbol column since it is defined
// as a stand-alone struct in the row.
type ColumnType int

const (
	// ColumnTypeBool defines a boolean column.
	ColumnTypeBool ColumnType = iota
	// ColumnTypeInt defines an integer column.
	ColumnTypeInt
	// ColumnTypeLong defines a long integer column.
	ColumnTypeLong
	// ColumnTypeFloat defines a float column.
	ColumnTypeFloat
	// ColumnTypeString defines a string column.
	ColumnTypeString
	// ColumnTypeTimestamp defines a timestamp column.
	ColumnTypeTimestamp
)

// Column represents a column of a row.
type Column struct {
	name  string
	typ   ColumnType
	value any
}

func newColumn(name string, typ ColumnType, value any) Column {
	return Column{
		name:  name,
		typ:   typ,
		value: value,
	}
}

// NewBoolColumn returns a new boolean column.
func NewBoolColumn(name string, value bool) Column {
	return newColumn(name, ColumnTypeBool, value)
}

// NewIntColumn returns a new integer column.
func NewIntColumn(name string, value int64) Column {
	return newColumn(name, ColumnTypeInt, value)
}

// NewLongColumn returns a new long integer column.
func NewLongColumn(name string, value *big.Int) Column {
	return newColumn(name, ColumnTypeLong, value)
}

// NewFloatColumn returns a new float column.
func NewFloatColumn(name string, value float64) Column {
	return newColumn(name, ColumnTypeFloat, value)
}

// NewStringColumn returns a new string column.
func NewStringColumn(name string, value string) Column {
	return newColumn(name, ColumnTypeString, value)
}

// NewTimestampColumn returns a new timestamp column.
func NewTimestampColumn(name string, value time.Time) Column {
	return newColumn(name, ColumnTypeTimestamp, value)
}

// GetName returns the name of the column.
func (c Column) GetName() string {
	return c.name
}

// GetType returns the type of the column.
func (c Column) GetType() ColumnType {
	return c.typ
}

// GetValue returns the value of the column.
func (c Column) GetValue() any {
	return c.value
}

// Symbol represents a symbol column.
// It is defined as a stand-alone struct because a symbol column
// must be inserted before any other column.
type Symbol struct {
	name  string
	value string
}

// NewSymbol returns a new symbol.
func NewSymbol(name string, value string) Symbol {
	return Symbol{
		name:  name,
		value: value,
	}
}

// GetName returns the name of the symbol.
func (s Symbol) GetName() string {
	return s.name
}

// GetValue returns the value of the symbol.
func (s Symbol) GetValue() string {
	return s.value
}

// Row represents a row to be inserted into the database.
type Row struct {
	table   string
	symbols []Symbol
	columns []Column
}

// NewRow returns a new row.
func NewRow(table string) *Row {
	return &Row{
		table: table,
	}
}

// AddSymbol adds a symbol to the row.
func (r *Row) AddSymbol(symbol Symbol) {
	r.symbols = append(r.symbols, symbol)
}

// AddSymbols adds multiple symbols to the row.
func (r *Row) AddSymbols(symbols ...Symbol) {
	if len(r.symbols) == 0 {
		r.symbols = symbols
		return
	}

	r.symbols = append(r.symbols, symbols...)
}

// AddColumn adds a column to the row.
func (r *Row) AddColumn(column Column) {
	r.columns = append(r.columns, column)
}

// AddColumns adds multiple columns to the row.
func (r *Row) AddColumns(columns ...Column) {
	if len(r.columns) == 0 {
		r.columns = columns
		return
	}

	r.columns = append(r.columns, columns...)
}
