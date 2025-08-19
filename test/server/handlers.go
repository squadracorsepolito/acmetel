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

func (h *rawHandler) Handle(_ context.Context, msg *can.Message) (*questdb.Message, error) {
	nextMsg := questdb.NewMessage()
	nextMsg.Timestamp = msg.GetTimestamp()

	rows := make([]*questdb.Row, 0, msg.SignalCount)

	for _, sig := range msg.Signals {
		valType := sig.Type

		row := questdb.NewRow(h.getTable(valType))

		row.AddSymbol(questdb.NewSymbol("name", sig.Name))

		row.AddColumn(questdb.NewIntColumn("can_id", sig.CANID))
		row.AddColumn(questdb.NewIntColumn("raw_value", int64(sig.RawValue)))

		switch valType {
		case can.ValueTypeFlag:
			row.AddColumn(questdb.NewBoolColumn("flag_value", sig.ValueFlag))

		case can.ValueTypeInt:
			row.AddColumn(questdb.NewIntColumn("integer_value", sig.ValueInt))

		case can.ValueTypeFloat:
			row.AddColumn(questdb.NewFloatColumn("float_value", sig.ValueFloat))

		case can.ValueTypeEnum:
			row.AddSymbol(questdb.NewSymbol("enum_value", sig.ValueEnum))
		}

		rows = append(rows, row)
	}

	nextMsg.AddRows(rows...)

	return nextMsg, nil
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
