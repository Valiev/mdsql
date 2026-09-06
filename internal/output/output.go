package output

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Valiev/mdsql/internal/query"
)

type Format string

const (
	FormatText    Format = "text"
	FormatJSON    Format = "json"
	FormatJSONL   Format = "jsonl"
	FormatCSV     Format = "csv"
	FormatTSV     Format = "tsv"
	FormatMD      Format = "md"
	FormatGrouped Format = "grouped"
)

type Options struct {
	NoHeader    bool
	Null        *string
	SchemaOrder bool
}

type Renderer struct {
	Out      io.Writer
	Format   Format
	MaxLines int
	WithFile bool
	NoFile   bool
	KindName string
	NoHeader bool
	Null     *string
}

func Render(w io.Writer, rows []query.Row, format Format, maxLines int, withFile bool, noFile bool, kind string, table []string, options ...Options) error {
	var opts Options
	if len(options) > 0 {
		opts = options[0]
	}
	if opts.NoHeader && format != FormatCSV && format != FormatTSV {
		return fmt.Errorf("--no-header requires csv or tsv format")
	}
	if err := ValidateProjection(format, table); err != nil {
		return err
	}
	if err := validateFormatRows(format, rows); err != nil {
		return err
	}
	r := Renderer{Out: w, Format: format, MaxLines: maxLines, WithFile: withFile, NoFile: noFile, KindName: kind, NoHeader: opts.NoHeader, Null: opts.Null}
	if opts.SchemaOrder && (format == FormatJSON || format == FormatJSONL) {
		return r.renderOrderedJSON(rows, table, format == FormatJSONL)
	}
	switch format {
	case FormatJSON:
		return r.renderJSON(rows)
	case FormatJSONL:
		return r.renderJSONL(rows)
	case FormatCSV:
		return r.renderCSV(rows, table)
	case FormatTSV:
		return r.renderTSV(rows, table)
	case FormatMD:
		return r.renderMD(rows)
	case FormatGrouped:
		return r.renderGrouped(rows, table)
	case Format("plain"):
		return r.renderPlain(rows, table)
	case Format("outline"):
		return r.renderOutline(rows)
	default:
		return r.renderText(rows, table)
	}
}

func (r *Renderer) renderText(rows []query.Row, keys []string) error {
	if len(rows) == 0 {
		return nil
	}
	bw := bufio.NewWriter(r.Out)

	for _, row := range rows {
		if row == nil {
			bw.WriteString("\n")
			continue
		}

		keyOrder := keys
		if len(keyOrder) == 0 || (len(keyOrder) == 1 && keyOrder[0] == "*") {
			keyOrder = nil
			for k := range row {
				if strings.HasPrefix(k, "__") {
					continue
				}
				keyOrder = append(keyOrder, k)
			}
			sort.Strings(keyOrder)
		}

		nonScalar := map[string]bool{}
		for _, k := range keyOrder {
			v := r.displayValue(row[k], "-")
			if v.Kind == query.ValueString && strings.Contains(v.String(), "\n") {
				nonScalar[k] = true
			}
		}

		firstNS := ""
		wroteScalar := false
		for _, k := range keyOrder {
			if k == "*" || strings.HasPrefix(k, "__") {
				continue
			}
			v := r.displayValue(row[k], "-")
			if nonScalar[k] {
				if firstNS == "" {
					firstNS = k
				}
				continue
			}
			if wroteScalar {
				bw.WriteString("\t")
			}
			fmt.Fprintf(bw, "%s=%s", k, scalarToHumanText(v))
			wroteScalar = true
		}
		if wroteScalar {
			bw.WriteString("\n")
		}
		if firstNS != "" {
			tmp := r.displayValue(row[firstNS], "-")
			printMultiLine(bw, firstNS, tmp, r.MaxLines)
		}
		for _, k := range keyOrder {
			v := r.displayValue(row[k], "-")
			if strings.HasPrefix(k, "__") || !nonScalar[k] || k == firstNS {
				continue
			}
			printMultiLine(bw, k, v, r.MaxLines)
		}
	}
	return bw.Flush()
}

func printMultiLine(w *bufio.Writer, key string, v query.Value, max int) {
	fmt.Fprintf(w, "%s:\n", key)
	lines := strings.Split(v.String(), "\n")
	if max > 0 && len(lines) > max {
		extra := len(lines) - max
		lines = lines[:max]
		for _, l := range lines {
			w.WriteString(strings.ReplaceAll(l, "\t", "\\t"))
			w.WriteString("\n")
		}
		fmt.Fprintf(w, "… (+%d lines)\n", extra)
		return
	}
	for _, l := range lines {
		w.WriteString(strings.ReplaceAll(l, "\t", "\\t"))
		w.WriteString("\n")
	}
}

func (r *Renderer) renderJSON(rows []query.Row) error {
	enc := json.NewEncoder(r.Out)
	res := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		res = append(res, serializeRow(row))
	}
	enc.SetIndent("", "")
	return enc.Encode(res)
}

func (r *Renderer) renderJSONL(rows []query.Row) error {
	enc := json.NewEncoder(r.Out)
	for _, row := range rows {
		if err := enc.Encode(serializeRow(row)); err != nil {
			return err
		}
	}
	return nil
}

func (r *Renderer) renderCSV(rows []query.Row, selected []string) error {
	if len(rows) == 0 {
		return nil
	}
	keys := selectedKeys(rows[0], selected)
	writer := csv.NewWriter(r.Out)
	if !r.NoHeader {
		if err := writer.Write(keys); err != nil {
			return err
		}
	}
	for _, row := range rows {
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, scalarToText(r.displayValue(row[key], "")))
		}
		if err := writer.Write(values); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

func selectedKeys(row query.Row, selected []string) []string {
	if len(selected) > 0 && !(len(selected) == 1 && selected[0] == "*") {
		return append([]string(nil), selected...)
	}
	keys := make([]string, 0, len(row))
	for key := range row {
		if !strings.HasPrefix(key, "__") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func (r *Renderer) renderTSV(rows []query.Row, selected []string) error {
	if len(rows) == 0 {
		return nil
	}
	keys := selectedKeys(rows[0], selected)
	wr := csv.NewWriter(r.Out)
	wr.Comma = '\t'
	if !r.NoHeader {
		if err := wr.Write(keys); err != nil {
			return err
		}
	}
	for _, row := range rows {
		vals := make([]string, 0, len(keys))
		for _, k := range keys {
			vals = append(vals, encodeForTSV(r.displayValue(row[k], "")))
		}
		if err := wr.Write(vals); err != nil {
			return err
		}
	}
	wr.Flush()
	return wr.Error()
}

func (r *Renderer) renderMD(rows []query.Row) error {
	bw := bufio.NewWriter(r.Out)
	for _, row := range rows {
		for _, k := range []string{"raw", "body", "own", "code"} {
			if v, ok := row[k]; ok {
				if v.Kind == query.ValueString {
					bw.WriteString(v.String())
					bw.WriteString("\n\n")
				}
			}
		}
	}
	return bw.Flush()
}

func (r *Renderer) renderPlain(rows []query.Row, selected []string) error {
	if len(rows) == 0 {
		return nil
	}
	keys := selectedKeys(rows[0], selected)
	bw := bufio.NewWriter(r.Out)
	for _, row := range rows {
		values := make([]string, 0, len(keys))
		for _, key := range keys {
			values = append(values, scalarToHumanText(r.displayValue(row[key], "")))
		}
		fmt.Fprintln(bw, strings.Join(values, "\t"))
	}
	return bw.Flush()
}

func (r *Renderer) renderOutline(rows []query.Row) error {
	bw := bufio.NewWriter(r.Out)
	for _, row := range rows {
		level := 1
		if value := row["level"]; value.Kind == query.ValueInt {
			level = int(value.Data.(int64))
		}
		if level < 1 {
			level = 1
		}
		fmt.Fprintf(bw, "%s- %s\n", strings.Repeat("  ", level-1), row["title"].String())
	}
	return bw.Flush()
}

func (r *Renderer) displayValue(v query.Value, nullText string) query.Value {
	if v.Kind != query.ValueNull {
		return v
	}
	if r.Null != nil {
		nullText = *r.Null
	}
	return query.NewString(nullText)
}

func encodeForTSV(v query.Value) string {
	s := scalarToText(v)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

func scalarToText(v query.Value) string {
	switch v.Kind {
	case query.ValueNull:
		return "-"
	case query.ValueBool:
		if v.Data.(bool) {
			return "true"
		}
		return "false"
	case query.ValueInt:
		return fmt.Sprintf("%d", v.Data.(int64))
	case query.ValueDate:
		return v.Data.(time.Time).Format(time.DateOnly)
	case query.ValueDateTime:
		return v.Data.(time.Time).Format(time.RFC3339)
	case query.ValueList, query.ValueObject:
		b, _ := json.Marshal(toAny(v))
		return string(b)
	default:
		return fmt.Sprintf("%v", v.String())
	}
}

func scalarToHumanText(v query.Value) string {
	if v.Kind != query.ValueFloat {
		return scalarToText(v)
	}
	formatted := strconv.FormatFloat(v.Data.(float64), 'f', 2, 64)
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	if formatted == "-0" {
		return "0"
	}
	return formatted
}

func serializeRow(row query.Row) map[string]any {
	res := map[string]any{}
	for k, v := range row {
		if strings.HasPrefix(k, "__") {
			continue
		}
		res[k] = toAny(v)
	}
	return res
}

func isLocationKey(key string) bool {
	switch key {
	case "file", "start", "end", "line", "n", "kind":
		return true
	default:
		return false
	}
}

func toAny(v query.Value) any {
	switch v.Kind {
	case query.ValueNull:
		return nil
	case query.ValueString:
		return v.Data.(string)
	case query.ValueInt:
		return v.Data.(int64)
	case query.ValueFloat:
		return v.Data.(float64)
	case query.ValueBool:
		return v.Data.(bool)
	case query.ValueDate:
		return v.Data.(time.Time).Format(time.DateOnly)
	case query.ValueDateTime:
		return v.Data.(time.Time).Format(time.RFC3339)
	case query.ValueList:
		arr := []any{}
		for _, e := range v.Data.([]query.Value) {
			arr = append(arr, toAny(e))
		}
		return arr
	case query.ValueObject:
		return serializeObject(v.Data.(map[string]query.Value))
	default:
		return v.String()
	}
}

func serializeObject(in map[string]query.Value) map[string]any {
	res := map[string]any{}
	for k, v := range in {
		res[k] = toAny(v)
	}
	return res
}
