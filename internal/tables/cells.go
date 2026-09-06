package tables

import (
	"strings"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

var cellsTable = TableDef{
	Name: "cells",
	Doc:  "Ячейки Markdown-таблиц",
	Columns: []query.Column{
		{Name: "file", Type: "string"},
		{Name: "table", Type: "int", Doc: "Номер таблицы в файле, начиная с 1"},
		{Name: "row", Type: "int", Doc: "0 для заголовка, данные начиная с 1"},
		{Name: "col", Type: "int", Doc: "Номер столбца, начиная с 1"},
		{Name: "header", Type: "string"},
		{Name: "value", Type: "string"},
		{Name: "is_header", Type: "bool"},
	},
	Rows: rowsCells,
}

func rowsCells(doc *model.Document) []query.Row {
	rows := []query.Row{}
	table := 0
	for _, block := range doc.Blocks {
		if block.Kind != model.KindTable {
			continue
		}
		headers, lines := markdownTable(block.Raw)
		if headers == nil {
			continue
		}
		table++
		row := 0
		for i, line := range lines {
			if i == 1 {
				continue
			}
			values := splitTableRow(line)
			for col, header := range headers {
				value := ""
				if col < len(values) {
					value = values[col]
				}
				rows = append(rows, query.Row{
					"file":      query.NewString(doc.Path),
					"table":     query.NewInt(int64(table)),
					"row":       query.NewInt(int64(row)),
					"col":       query.NewInt(int64(col + 1)),
					"header":    query.NewString(header),
					"value":     query.NewString(value),
					"is_header": query.NewBool(row == 0),
				})
			}
			row++
		}
	}
	return rows
}

// markdownTable shares header validation between cells and table metrics.
func markdownTable(raw string) (headers, lines []string) {
	lines = strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 2 {
		return nil, nil
	}
	headers = splitTableRow(lines[0])
	delimiter := splitTableRow(lines[1])
	if len(headers) != len(delimiter) || !isTableDelimiter(delimiter) {
		return nil, nil
	}
	return headers, lines
}

func splitTableRow(line string) []string {
	line = strings.TrimSpace(line)
	var cells []string
	var cell strings.Builder
	trailingPipe := false
	for i := 0; i < len(line); i++ {
		trailingPipe = false
		if line[i] == '\\' && i+1 < len(line) {
			if line[i+1] != '|' {
				cell.WriteByte(line[i])
			}
			i++
			cell.WriteByte(line[i])
			continue
		}
		if line[i] == '|' {
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
			trailingPipe = true
			continue
		}
		cell.WriteByte(line[i])
	}
	if !trailingPipe {
		cells = append(cells, strings.TrimSpace(cell.String()))
	}
	if strings.HasPrefix(line, "|") {
		cells = cells[1:]
	}
	return cells
}

func isTableDelimiter(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		cell = strings.TrimSuffix(strings.TrimPrefix(cell, ":"), ":")
		if cell == "" || strings.Trim(cell, "-") != "" {
			return false
		}
	}
	return true
}
