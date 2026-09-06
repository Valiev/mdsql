package tables

import (
	"reflect"
	"testing"

	"github.com/Valiev/mdsql/internal/model"
)

func TestSplitTableRowEscapedPipesAndBorders(t *testing.T) {
	for _, tc := range []struct {
		line string
		want []string
	}{
		{`| Name | Workflow |`, []string{"Name", "Workflow"}},
		{` Name | Workflow `, []string{"Name", "Workflow"}},
		{`| A\|B | C |`, []string{"A|B", "C"}},
		{`| A\|`, []string{"A|"}},
		{`| A\\| B |`, []string{`A\\`, "B"}},
		{`| A\\\|B |`, []string{`A\\|B`}},
		{`| | Value | |`, []string{"", "Value", ""}},
		{`| **Bold** | [link](url) |`, []string{"**Bold**", "[link](url)"}},
		{`| Имя | Процесс |`, []string{"Имя", "Процесс"}},
	} {
		t.Run(tc.line, func(t *testing.T) {
			if got := splitTableRow(tc.line); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("cells=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestCellsHeaderNumberingAndRaggedRows(t *testing.T) {
	doc := &model.Document{Path: "synthetic.md", Blocks: []model.Block{
		{Kind: model.KindParagraph, Raw: "ignored"},
		{Kind: model.KindTable, Raw: "| Name | Workflow |\n| :--- | ---: |\n| First | build |\n| Missing |\n| Extra | test | ignored |"},
		{Kind: model.KindTable, Raw: "| Workflow |\n| --- |\n| deploy |"},
	}}
	rows := rowsCells(doc)
	want := []struct {
		table, row, col int
		header, value   string
	}{
		{1, 0, 1, "Name", "Name"}, {1, 0, 2, "Workflow", "Workflow"},
		{1, 1, 1, "Name", "First"}, {1, 1, 2, "Workflow", "build"},
		{1, 2, 1, "Name", "Missing"}, {1, 2, 2, "Workflow", ""},
		{1, 3, 1, "Name", "Extra"}, {1, 3, 2, "Workflow", "test"},
		{2, 0, 1, "Workflow", "Workflow"}, {2, 1, 1, "Workflow", "deploy"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows=%v", rows)
	}
	for i, expected := range want {
		row := rows[i]
		if row["is_header"].Data != (expected.row == 0) {
			t.Errorf("row %d is_header=%v", i, row["is_header"])
		}
		if row["file"].String() != doc.Path || row["table"].Data != int64(expected.table) || row["row"].Data != int64(expected.row) || row["col"].Data != int64(expected.col) || row["header"].String() != expected.header || row["value"].String() != expected.value {
			t.Errorf("row %d=%v want=%+v", i, row, expected)
		}
	}
}

func TestTableMetricsAndHeaderOnlyRows(t *testing.T) {
	table, ok := Table("tables")
	if !ok {
		t.Fatal("missing tables")
	}
	for _, tc := range []struct {
		raw           string
		rows, columns int64
	}{
		{"| A | B | C |\n| :--- | :---: | ---: |\n| | | |\n| One |", 2, 3},
		{"A | B\n--- | ---", 0, 2},
		{"| A\\|B | C |\n| --- | --- |\n| x | y | extra |", 1, 2},
		{"| A |\n| invalid |", 0, 0},
	} {
		doc := &model.Document{Blocks: []model.Block{{Kind: model.KindTable, Raw: tc.raw}}}
		rows := table.Rows(doc)
		if len(rows) != 1 || rows[0]["rows"].Data != tc.rows || rows[0]["columns"].Data != tc.columns {
			t.Fatalf("table %q: %v", tc.raw, rows)
		}
		cells := rowsCells(doc)
		if int64(len(cells)) != (tc.rows+1)*tc.columns {
			t.Errorf("table %q: cells=%d", tc.raw, len(cells))
		}
	}
}

func TestCellsRejectsInvalidTableBlocks(t *testing.T) {
	for _, raw := range []string{"", "| Header |", "| Header |\n| ordinary text |", "| One | Two |\n| --- |", "| One |\n| :-x-: |"} {
		rows := rowsCells(&model.Document{Blocks: []model.Block{{Kind: model.KindTable, Raw: raw}}})
		if len(rows) != 0 {
			t.Errorf("raw=%q rows=%v", raw, rows)
		}
	}
}
