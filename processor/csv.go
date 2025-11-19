package processor

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	stageCommon "github.com/squadracorsepolito/acmetel/internal/stage"
)

//////////////
//  CONFIG  //
//////////////

type CSVValueType uint8

const (
	CSVValueTypeString CSVValueType = iota
	CSVValueTypeInt
	CSVValueTypeFloat
	CSVValueTypeBool
	CSVValueTypeTimestamp
)

type CSVColumn struct {
	Name           string
	Type           CSVValueType
	IsDataValid    bool
	StringValue    string
	IntValue       int64
	FloatValue     float64
	BoolValue      bool
	TimestampValue time.Time
}

type CSVColumnDef struct {
	Name            string
	Type            CSVValueType
	TimestampLayout string
}

type CSVConfig struct {
	Stage *stageCommon.Config

	Columns []*CSVColumnDef
}

///////////////
//  DECODER  //
///////////////

var csvDecoderBoolTrueIdents = map[string]struct{}{
	"true": {},
	"1":    {},
	"yes":  {},
}

var csvDecoderBoolFalseIdents = map[string]struct{}{
	"false": {},
	"0":     {},
	"no":    {},
}

type csvDecoder struct {
	columnDefs  []*CSVColumnDef
	columnCount int
}

func newCSVDecoder(config *CSVConfig) *csvDecoder {
	return &csvDecoder{
		columnDefs:  config.Columns,
		columnCount: len(config.Columns),
	}
}

func (cd *csvDecoder) decode(data []byte) ([][]*CSVColumn, error) {
	acc := &strings.Builder{}

	rows := make([][]*CSVColumn, 0, 128)

	row := make([]*CSVColumn, cd.columnCount)
	columnIdx := 0

	idx := 0
	for idx < len(data) {
		if columnIdx == 0 {
			if idx != 0 {
				// Finished a row
				rows = append(rows, row)
			}

			// Start a new row
			row = make([]*CSVColumn, cd.columnCount)
		}

		r, size := utf8.DecodeRune(data[idx:])
		idx += size

		switch r {
		case ',', '\n':
			// Found a column
			col := cd.decodeColumn(acc.String(), columnIdx)

			row[columnIdx] = col
			columnIdx++

			// Reached end of row
			if columnIdx == cd.columnCount {
				// Check if new line symbol is valid.
				// Consider that the last row may not end with a new line.
				if r != '\n' && idx < len(data) {
					// Invalid CSV format, expected new line
					return nil, errors.New("invalid CSV format: expected new line")
				}

				columnIdx = 0
				acc.Reset()
				rows = append(rows, row)
			}

		case '\r':
			// Ignore carriage return
			continue

		default:
			acc.WriteRune(r)
		}
	}

	// Check if the last row is incomplete
	if columnIdx != 0 {
		return nil, errors.New("invalid CSV format: incomplete row at the end")
	}

	return rows, nil
}

func (cd *csvDecoder) initColumnValue(columnIdx int) *CSVColumn {
	return &CSVColumn{
		Name: cd.columnDefs[columnIdx].Name,
		Type: cd.columnDefs[columnIdx].Type,
	}
}

func (cd *csvDecoder) decodeColumn(columnData string, columnIdx int) *CSVColumn {
	column := cd.initColumnValue(columnIdx)
	column.IsDataValid = true

	switch column.Type {
	case CSVValueTypeString:
		cd.decodeString(column, columnData)

	case CSVValueTypeInt:
		cd.decodeInt(column, columnData)

	case CSVValueTypeFloat:
		cd.decodeFloat(column, columnData)

	case CSVValueTypeBool:
		cd.decodeBool(column, columnData)

	case CSVValueTypeTimestamp:
		cd.decodeTimestamp(column, columnData, columnIdx)
	}

	return column
}

func (cd *csvDecoder) decodeString(column *CSVColumn, data string) {
	column.StringValue = data
}

func (cd *csvDecoder) decodeInt(column *CSVColumn, data string) {
	intVal, err := strconv.Atoi(data)
	if err != nil {
		column.IsDataValid = false
		return
	}

	column.IntValue = int64(intVal)
}

func (cd *csvDecoder) decodeFloat(column *CSVColumn, data string) {
	floatVal, err := strconv.ParseFloat(data, 64)
	if err != nil {
		column.IsDataValid = false
		return
	}

	column.FloatValue = floatVal
}

func (cd *csvDecoder) decodeBool(column *CSVColumn, data string) {
	column.BoolValue = false

	if _, ok := csvDecoderBoolTrueIdents[data]; ok {
		column.BoolValue = true
		return
	}

	if _, ok := csvDecoderBoolFalseIdents[data]; ok {
		return
	}

	dataLower := strings.ToLower(data)
	if _, ok := csvDecoderBoolTrueIdents[dataLower]; ok {
		column.BoolValue = true
		return
	}

	if _, ok := csvDecoderBoolFalseIdents[dataLower]; ok {
		return
	}

	column.IsDataValid = false
}

func (cd *csvDecoder) getCurrentTimestampLayout(currColumn int) string {
	layout := cd.columnDefs[currColumn].TimestampLayout
	if layout == "" {
		layout = time.RFC3339
	}
	return layout
}

func (cd *csvDecoder) decodeTimestamp(column *CSVColumn, data string, currColumn int) {
	timeValue, err := time.Parse(cd.getCurrentTimestampLayout(currColumn), data)
	if err != nil {
		column.IsDataValid = false
		return
	}

	column.TimestampValue = timeValue
}
