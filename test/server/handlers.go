package main

import (
	"context"

	"github.com/squadracorsepolito/acmetel/processor"
	"github.com/squadracorsepolito/acmetel/questdb"
)

type canToQuestDBHandler struct{}

func newCANToQuestDBHandler() *canToQuestDBHandler {
	return &canToQuestDBHandler{}
}

func (h *canToQuestDBHandler) Init(_ context.Context) error {
	return nil
}

func (h *canToQuestDBHandler) Handle(_ context.Context, canMsg *processor.CANMessage, qdbMsg *questdb.Message) error {
	rows := make([]*questdb.Row, 0, canMsg.SignalCount)

	for _, sig := range canMsg.Signals {
		valType := sig.Type

		row := questdb.NewRow(h.getTable(valType))

		row.AddSymbol(questdb.NewSymbol("name", sig.Name))

		columns := make([]questdb.Column, 0, 3)

		columns = append(columns, questdb.NewIntColumn("can_id", int64(sig.CANID)))
		columns = append(columns, questdb.NewIntColumn("raw_value", int64(sig.RawValue)))

		switch valType {
		case processor.CANSignalValueTypeFlag:
			columns = append(columns, questdb.NewBoolColumn("flag_value", sig.ValueFlag))

		case processor.CANSignalValueTypeInt:
			columns = append(columns, questdb.NewIntColumn("integer_value", sig.ValueInt))

		case processor.CANSignalValueTypeFloat:
			columns = append(columns, questdb.NewFloatColumn("float_value", sig.ValueFloat))

		case processor.CANSignalValueTypeEnum:
			row.AddSymbol(questdb.NewSymbol("enum_value", sig.ValueEnum))
		}

		row.AddColumns(columns...)
		rows = append(rows, row)
	}

	qdbMsg.AddRows(rows...)

	return nil
}

func (h *canToQuestDBHandler) Close() {}

func (h *canToQuestDBHandler) getTable(valType processor.CANSignalValueType) string {
	switch valType {
	case processor.CANSignalValueTypeFlag:
		return "flag_signals"
	case processor.CANSignalValueTypeInt:
		return "int_signals"
	case processor.CANSignalValueTypeFloat:
		return "float_signals"
	case processor.CANSignalValueTypeEnum:
		return "enum_signals"
	default:
		return "unknown_signals"
	}
}
