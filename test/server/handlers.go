package main

import (
	"context"

	"github.com/squadracorsepolito/acmetel/can"
	"github.com/squadracorsepolito/acmetel/questdb"
)

type rawHandler struct{}

func newRawHandler() *rawHandler {
	return &rawHandler{}
}

func (h *rawHandler) Init(_ context.Context) error {
	return nil
}

func (h *rawHandler) Handle(_ context.Context, canMsg *can.Message, qdbMsg *questdb.Message) error {
	rows := make([]*questdb.Row, 0, canMsg.SignalCount)

	for _, sig := range canMsg.Signals {
		valType := sig.Type

		row := questdb.NewRow(h.getTable(valType))

		row.AddSymbol(questdb.NewSymbol("name", sig.Name))

		columns := make([]questdb.Column, 0, 3)

		columns = append(columns, questdb.NewIntColumn("can_id", int64(sig.CANID)))
		columns = append(columns, questdb.NewIntColumn("raw_value", int64(sig.RawValue)))

		switch valType {
		case can.ValueTypeFlag:
			columns = append(columns, questdb.NewBoolColumn("flag_value", sig.ValueFlag))

		case can.ValueTypeInt:
			columns = append(columns, questdb.NewIntColumn("integer_value", sig.ValueInt))

		case can.ValueTypeFloat:
			columns = append(columns, questdb.NewFloatColumn("float_value", sig.ValueFloat))

		case can.ValueTypeEnum:
			row.AddSymbol(questdb.NewSymbol("enum_value", sig.ValueEnum))
		}

		row.AddColumns(columns...)
		rows = append(rows, row)
	}

	qdbMsg.AddRows(rows...)

	return nil
}

func (h *rawHandler) Close() {}

func (h *rawHandler) getTable(valType can.ValueType) string {
	switch valType {
	case can.ValueTypeFlag:
		return "flag_signals"
	case can.ValueTypeInt:
		return "int_signals"
	case can.ValueTypeFloat:
		return "float_signals"
	case can.ValueTypeEnum:
		return "enum_signals"
	default:
		return "unknown_signals"
	}
}
