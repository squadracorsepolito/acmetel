package main

import (
	"context"

	"github.com/squadracorsepolito/acmetel/can"
	"github.com/squadracorsepolito/acmetel/questdb"
)

type questDBHandler struct{}

func (h *questDBHandler) Init(_ context.Context) error {
	return nil
}

func (h *questDBHandler) Handle(_ context.Context, msg *can.Message) (*questdb.Result, error) {
	rows := make([]*questdb.Row, 0, msg.SignalCount)

	for _, sig := range msg.Signals {
		table := sig.Table

		row := questdb.NewRow(table.String())

		row.AddColumn(questdb.NewSymbolColumn("name", sig.Name))
		row.AddColumn(questdb.NewIntColumn("can_id", sig.CANID))
		row.AddColumn(questdb.NewIntColumn("raw_value", sig.RawValue))

		switch table {
		case can.CANSignalTableFlag:
			row.AddColumn(questdb.NewBoolColumn("flag_value", sig.ValueFlag))

		case can.CANSignalTableInt:
			row.AddColumn(questdb.NewIntColumn("integer_value", sig.ValueInt))

		case can.CANSignalTableFloat:
			row.AddColumn(questdb.NewFloatColumn("float_value", sig.ValueFloat))

		case can.CANSignalTableEnum:
			row.AddColumn(questdb.NewSymbolColumn("enum_value", sig.ValueEnum))
		}

		rows = append(rows, row)
	}

	return questdb.NewResult(rows...), nil
}

func (h *questDBHandler) Close() {}
