package engine

import (
	"reflect"
	"testing"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
	"github.com/Valiev/mdsql/internal/tables"
)

func TestStarUsesDeclaredColumnOrderWithoutFrontmatter(t *testing.T) {
	doc := &model.Document{
		Path:        "synthetic.md",
		FrontMatter: model.FrontMatter{"label": "demo"},
		Headings:    []model.Heading{{File: "synthetic.md", Title: "Demo", Start: 1}},
	}
	for _, name := range []string{"headings", "files", "frontmatter"} {
		t.Run(name, func(t *testing.T) {
			q, err := query.Parse("SELECT * FROM " + name)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Execute(q, []*model.Document{doc}, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Rows) != 1 {
				t.Fatalf("rows=%v", result.Rows)
			}
			table, _ := tables.Table(name)
			want := starColumns(table)
			if !reflect.DeepEqual(result.Columns, want) {
				t.Fatalf("columns=%v want=%v", result.Columns, want)
			}
			row := result.Rows[0]
			if len(row) != len(want) {
				t.Fatalf("row=%v want columns=%v", row, want)
			}
			for _, column := range want {
				if _, ok := row[column]; !ok {
					t.Errorf("missing column %q", column)
				}
			}
			if _, hasFM := row["fm"]; hasFM != (name == "frontmatter") {
				t.Fatalf("row contains fm=%v", hasFM)
			}
			if !reflect.DeepEqual(q.SelectKeys(), []string{"*"}) {
				t.Fatalf("Execute mutated SELECT: %v", q.SelectKeys())
			}
		})
	}
}

func TestEmptyStarHasSchemaAndExplicitFrontmatter(t *testing.T) {
	for _, table := range tables.Registry {
		t.Run(table.Name, func(t *testing.T) {
			q, err := query.Parse("SELECT * FROM " + table.Name)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Execute(q, nil, Options{})
			if err != nil || len(result.Rows) != 0 || !reflect.DeepEqual(result.Columns, starColumns(table)) {
				t.Fatalf("result=%#v error=%v", result, err)
			}
			for _, column := range table.Columns {
				if column.Name != "fm" {
					continue
				}
				q, err = query.Parse("SELECT fm FROM " + table.Name)
				if err != nil {
					t.Fatal(err)
				}
				result, err = Execute(q, nil, Options{})
				if err != nil || !reflect.DeepEqual(result.Columns, []string{"fm"}) {
					t.Fatalf("explicit fm result=%#v error=%v", result, err)
				}
			}
		})
	}
}

func TestStarWithAliasesAndExplicitFrontmatter(t *testing.T) {
	doc := &model.Document{
		FrontMatter: model.FrontMatter{"label": "demo"},
		Headings:    []model.Heading{{Title: "Demo"}},
	}
	table, _ := tables.Table("headings")
	for _, tc := range []struct {
		selectSQL string
		columns   []string
	}{
		{"*, fm", append(starColumns(table), "fm")},
		{"upper(title) AS label, *, len(title)", append(append([]string{"label"}, starColumns(table)...), "len")},
		{"fm, title AS label", []string{"fm", "label"}},
	} {
		q, err := query.Parse("SELECT " + tc.selectSQL + " FROM headings")
		if err != nil {
			t.Fatal(err)
		}
		result, err := Execute(q, []*model.Document{doc}, Options{})
		if err != nil || !reflect.DeepEqual(result.Columns, tc.columns) {
			t.Fatalf("SELECT %s: result=%#v error=%v", tc.selectSQL, result, err)
		}
		if len(result.Rows) != 1 {
			t.Fatalf("rows=%v", result.Rows)
		}
		for _, column := range tc.columns {
			if _, ok := result.Rows[0][column]; !ok {
				t.Errorf("SELECT %s: missing %q", tc.selectSQL, column)
			}
		}
		if value, ok := result.Rows[0]["fm"]; ok && value.Kind != query.ValueObject {
			t.Fatalf("explicit fm=%v", value)
		}
	}
}

func starColumns(table tables.TableDef) []string {
	var columns []string
	for _, column := range table.Columns {
		if column.Name != "fm" || table.Name == "frontmatter" {
			columns = append(columns, column.Name)
		}
	}
	return columns
}
