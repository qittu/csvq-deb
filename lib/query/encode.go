package query

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/mithrandie/csvq/lib/cmd"
	"github.com/mithrandie/csvq/lib/json"
	"github.com/mithrandie/csvq/lib/value"

	"github.com/mithrandie/go-text"
	"github.com/mithrandie/go-text/color"
	"github.com/mithrandie/go-text/csv"
	"github.com/mithrandie/go-text/fixedlen"
	txjson "github.com/mithrandie/go-text/json"
	"github.com/mithrandie/go-text/ltsv"
	"github.com/mithrandie/go-text/table"
	"github.com/mithrandie/ternary"
)

var EmptyResultSetError = errors.New("empty result set")
var DataEmpty = errors.New("data empty")

func EncodeView(ctx context.Context, fp io.Writer, view *View, options cmd.ExportOptions, palette *color.Palette) (string, error) {
	switch options.Format {
	case cmd.FIXED:
		return "", encodeFixedLengthFormat(ctx, fp, view, options)
	case cmd.JSON:
		return "", encodeJson(ctx, fp, view, options, palette)
	case cmd.LTSV:
		return "", encodeLTSV(ctx, fp, view, options)
	case cmd.GFM, cmd.ORG, cmd.TEXT:
		return encodeText(ctx, fp, view, options, palette)
	case cmd.TSV:
		options.Delimiter = '\t'
		fallthrough
	default: // cmd.CSV
		return "", encodeCSV(ctx, fp, view, options)
	}
}

func encodeCSV(ctx context.Context, fp io.Writer, view *View, options cmd.ExportOptions) error {
	w, err := csv.NewWriter(fp, options.LineBreak, options.Encoding)
	if err != nil {
		return NewDataEncodingError(err.Error())
	}
	w.Delimiter = options.Delimiter

	fields := make([]csv.Field, view.FieldLen())

	if !options.WithoutHeader {
		for i := range view.Header {
			fields[i] = csv.NewField(view.Header[i].Column, options.EncloseAll)
		}
		if err := w.Write(fields); err != nil {
			return NewSystemError(err.Error())
		}
	} else if view.RecordLen() < 1 {
		return DataEmpty
	}

	for i := range view.RecordSet {
		if i&15 == 0 && ctx.Err() != nil {
			err = ConvertContextError(ctx.Err())
			break
		}

		for j := range view.RecordSet[i] {
			str, effect, _ := ConvertFieldContents(view.RecordSet[i][j][0], false)
			quote := false
			if options.EncloseAll && (effect == cmd.StringEffect || effect == cmd.DatetimeEffect) {
				quote = true
			}
			fields[j] = csv.NewField(str, quote)
		}
		if err := w.Write(fields); err != nil {
			return NewSystemError(err.Error())
		}
	}
	if err = w.Flush(); err != nil {
		return NewSystemError(err.Error())
	}
	return nil
}

func encodeFixedLengthFormat(ctx context.Context, fp io.Writer, view *View, options cmd.ExportOptions) error {
	if options.DelimiterPositions == nil {
		m := fixedlen.NewMeasure()
		m.Encoding = options.Encoding

		var fieldList [][]fixedlen.Field = nil
		var recordStartPos = 0
		var fieldLen = view.FieldLen()

		if options.WithoutHeader {
			if view.RecordLen() < 1 {
				return DataEmpty
			}
			fieldList = make([][]fixedlen.Field, view.RecordLen())
		} else {
			fieldList = make([][]fixedlen.Field, view.RecordLen()+1)
			recordStartPos = 1

			fields := make([]fixedlen.Field, fieldLen)
			for i := range view.Header {
				fields[i] = fixedlen.NewField(view.Header[i].Column, text.NotAligned)
			}
			fieldList[0] = fields
			m.Measure(fields)
		}

		for i := range view.RecordSet {
			if i&15 == 0 && ctx.Err() != nil {
				return ConvertContextError(ctx.Err())
			}

			fields := make([]fixedlen.Field, fieldLen)
			for j := range view.RecordSet[i] {
				str, _, a := ConvertFieldContents(view.RecordSet[i][j][0], false)
				fields[j] = fixedlen.NewField(str, a)
			}
			fieldList[i+recordStartPos] = fields
			m.Measure(fields)
		}

		options.DelimiterPositions = m.GeneratePositions()
		w, err := fixedlen.NewWriter(fp, options.DelimiterPositions, options.LineBreak, options.Encoding)
		if err != nil {
			return NewDataEncodingError(err.Error())
		}
		w.InsertSpace = true
		for i := range fieldList {
			if i&15 == 0 && ctx.Err() != nil {
				return ConvertContextError(ctx.Err())
			}

			if err := w.Write(fieldList[i]); err != nil {
				return NewDataEncodingError(err.Error())
			}
		}
		if err = w.Flush(); err != nil {
			return NewSystemError(err.Error())
		}

	} else {
		w, err := fixedlen.NewWriter(fp, options.DelimiterPositions, options.LineBreak, options.Encoding)
		if err != nil {
			return NewDataEncodingError(err.Error())
		}
		w.SingleLine = options.SingleLine

		fields := make([]fixedlen.Field, view.FieldLen())

		if options.WithoutHeader {
			if view.RecordLen() < 1 {
				return DataEmpty
			}
		} else if !options.SingleLine {
			for i := range view.Header {
				fields[i] = fixedlen.NewField(view.Header[i].Column, text.NotAligned)
			}
			if err := w.Write(fields); err != nil {
				return NewDataEncodingError(err.Error())
			}
		}

		for i := range view.RecordSet {
			if i&15 == 0 && ctx.Err() != nil {
				return ConvertContextError(ctx.Err())
			}

			for j := range view.RecordSet[i] {
				str, _, a := ConvertFieldContents(view.RecordSet[i][j][0], false)
				fields[j] = fixedlen.NewField(str, a)
			}
			if err := w.Write(fields); err != nil {
				return NewDataEncodingError(err.Error())
			}
		}
		if err = w.Flush(); err != nil {
			return NewSystemError(err.Error())
		}
	}
	return nil
}

func encodeJson(ctx context.Context, fp io.Writer, view *View, options cmd.ExportOptions, palette *color.Palette) error {
	header := view.Header.TableColumnNames()
	records := make([][]value.Primary, view.RecordLen())
	for i := range view.RecordSet {
		if i&15 == 0 && ctx.Err() != nil {
			return ConvertContextError(ctx.Err())
		}

		row := make([]value.Primary, view.FieldLen())
		for j := range view.RecordSet[i] {
			row[j] = view.RecordSet[i][j][0]
		}
		records[i] = row
	}

	data, err := json.ConvertTableValueToJsonStructure(ctx, header, records)
	if err != nil {
		if ctx.Err() != nil {
			return ConvertContextError(ctx.Err())
		}
		return NewDataEncodingError(err.Error())
	}

	e := txjson.NewEncoder()
	e.EscapeType = options.JsonEscape
	e.LineBreak = options.LineBreak
	e.PrettyPrint = options.PrettyPrint
	if options.PrettyPrint && options.Color {
		e.Palette = palette
	}
	defer func() {
		if options.Color {
			palette.Enable()
		} else {
			palette.Disable()
		}
	}()

	s := e.Encode(data)

	w := bufio.NewWriter(fp)
	if _, err = w.WriteString(s); err != nil {
		return NewSystemError(err.Error())
	}
	if err = w.Flush(); err != nil {
		return NewSystemError(err.Error())
	}
	return nil
}

func encodeText(ctx context.Context, fp io.Writer, view *View, options cmd.ExportOptions, palette *color.Palette) (string, error) {
	isPlainTable := false

	var tableFormat = table.PlainTable
	switch options.Format {
	case cmd.GFM:
		tableFormat = table.GFMTable
	case cmd.ORG:
		tableFormat = table.OrgTable
	default:
		if view.FieldLen() < 1 {
			return "Empty Fields", EmptyResultSetError
		}
		if view.RecordLen() < 1 {
			return "Empty RecordSet", EmptyResultSetError
		}
		isPlainTable = true
	}

	e := table.NewEncoder(tableFormat, view.RecordLen())
	e.LineBreak = options.LineBreak
	e.EastAsianEncoding = options.EastAsianEncoding
	e.CountDiacriticalSign = options.CountDiacriticalSign
	e.CountFormatCode = options.CountFormatCode
	e.WithoutHeader = options.WithoutHeader
	e.Encoding = options.Encoding

	fieldLen := view.FieldLen()

	if !options.WithoutHeader {
		hfields := make([]table.Field, fieldLen)
		for i := range view.Header {
			hfields[i] = table.NewField(view.Header[i].Column, text.Centering)
		}
		e.SetHeader(hfields)
	} else if view.RecordLen() < 1 {
		return "", DataEmpty
	}

	aligns := make([]text.FieldAlignment, fieldLen)

	var textStrBuf bytes.Buffer
	var textLineBuf bytes.Buffer
	for i := range view.RecordSet {
		if i&15 == 0 && ctx.Err() != nil {
			return "", ConvertContextError(ctx.Err())
		}

		rfields := make([]table.Field, fieldLen)
		for j := range view.RecordSet[i] {
			str, effect, align := ConvertFieldContents(view.RecordSet[i][j][0], isPlainTable)
			if options.Format == cmd.TEXT {
				textStrBuf.Reset()
				textLineBuf.Reset()

				runes := []rune(str)
				pos := 0
				for {
					if len(runes) <= pos {
						if 0 < textLineBuf.Len() {
							textStrBuf.WriteString(palette.Render(effect, textLineBuf.String()))
						}
						break
					}

					r := runes[pos]
					switch r {
					case '\r':
						if (pos+1) < len(runes) && runes[pos+1] == '\n' {
							pos++
						}
						fallthrough
					case '\n':
						if 0 < textLineBuf.Len() {
							textStrBuf.WriteString(palette.Render(effect, textLineBuf.String()))
						}
						textStrBuf.WriteByte('\n')
						textLineBuf.Reset()
					default:
						textLineBuf.WriteRune(r)
					}

					pos++
				}
				str = textStrBuf.String()
			}
			rfields[j] = table.NewField(str, align)

			if i == 0 {
				aligns[j] = align
			}
		}
		e.AppendRecord(rfields)
	}

	if options.Format == cmd.GFM {
		e.SetFieldAlignments(aligns)
	}

	s, err := e.Encode()
	if err != nil {
		return "", NewDataEncodingError(err.Error())
	}
	w := bufio.NewWriter(fp)
	if _, err = w.WriteString(s); err != nil {
		return "", NewSystemError(err.Error())
	}
	if err = w.Flush(); err != nil {
		return "", NewSystemError(err.Error())
	}
	return "", nil
}

func encodeLTSV(ctx context.Context, fp io.Writer, view *View, options cmd.ExportOptions) error {
	if view.RecordLen() < 1 {
		return DataEmpty
	}

	hfields := make([]string, view.FieldLen())
	for i := range view.Header {
		hfields[i] = view.Header[i].Column
	}

	w, err := ltsv.NewWriter(fp, hfields, options.LineBreak, options.Encoding)
	if err != nil {
		return NewDataEncodingError(err.Error())
	}

	fields := make([]string, view.FieldLen())
	for i := range view.RecordSet {
		if i&15 == 0 && ctx.Err() != nil {
			return ConvertContextError(ctx.Err())
		}

		for j := range view.RecordSet[i] {
			fields[j], _, _ = ConvertFieldContents(view.RecordSet[i][j][0], false)
		}
		if err := w.Write(fields); err != nil {
			return NewDataEncodingError(err.Error())
		}
	}
	if err = w.Flush(); err != nil {
		return NewSystemError(err.Error())
	}
	return nil
}

func ConvertFieldContents(val value.Primary, forTextTable bool) (string, string, text.FieldAlignment) {
	var s string
	var effect = cmd.NoEffect
	var align = text.NotAligned

	switch val.(type) {
	case *value.String:
		s = val.(*value.String).Raw()
		effect = cmd.StringEffect
	case *value.Integer:
		s = val.(*value.Integer).String()
		effect = cmd.NumberEffect
		align = text.RightAligned
	case *value.Float:
		s = val.(*value.Float).String()
		effect = cmd.NumberEffect
		align = text.RightAligned
	case *value.Boolean:
		s = val.(*value.Boolean).String()
		effect = cmd.BooleanEffect
		align = text.Centering
	case *value.Ternary:
		t := val.(*value.Ternary)
		if forTextTable {
			s = t.Ternary().String()
			effect = cmd.TernaryEffect
			align = text.Centering
		} else if t.Ternary() != ternary.UNKNOWN {
			s = strconv.FormatBool(t.Ternary().ParseBool())
			effect = cmd.BooleanEffect
			align = text.Centering
		}
	case *value.Datetime:
		s = val.(*value.Datetime).Format(time.RFC3339Nano)
		effect = cmd.DatetimeEffect
	case *value.Null:
		if forTextTable {
			s = "NULL"
			effect = cmd.NullEffect
			align = text.Centering
		}
	}

	return s, effect, align
}
