package tables

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func TestFrontmatterValuesSchema(t *testing.T) {
	table, ok := Table("frontmatter_values")
	if !ok {
		t.Fatal("frontmatter_values is not registered")
	}
	want := []query.Column{
		{Name: "file", Type: "string", Doc: "Source path"},
		{Name: "key", Type: "string", Doc: "YAML key"},
		{Name: "value", Type: "any", Doc: "Field value"},
	}
	if !reflect.DeepEqual(table.Columns, want) {
		t.Fatalf("columns = %#v, want %#v", table.Columns, want)
	}
}

func TestFrontmatterValuesTypesAndOrder(t *testing.T) {
	doc := &model.Document{
		Path: "synthetic.md",
		FrontMatter: model.FrontMatter{
			"text":        "hello",
			"null":        nil,
			"int":         42,
			"int64":       int64(43),
			"float":       1.5,
			"bool":        true,
			"date":        "2026-09-02",
			"literal.key": "literal",
			"empty":       []interface{}{},
			"object":      map[string]interface{}{"stage": "plan"},
			"nested_fm":   model.FrontMatter{"stage": "verify"},
			"list": []interface{}{
				"beta", "alpha", "beta", nil, 7, true,
				[]interface{}{"nested", "list"},
				map[string]interface{}{"stage": "implement"},
			},
		},
	}
	want := []struct {
		key   string
		value query.Value
	}{
		{"bool", query.NewBool(true)},
		{"date", query.NewDate(time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))},
		{"float", query.NewFloat(1.5)},
		{"int", query.NewInt(42)},
		{"int64", query.NewInt(43)},
		{"list", query.NewString("beta")},
		{"list", query.NewString("alpha")},
		{"list", query.NewString("beta")},
		{"list", query.NewNull()},
		{"list", query.NewInt(7)},
		{"list", query.NewBool(true)},
		{"list", query.NewList([]query.Value{query.NewString("nested"), query.NewString("list")})},
		{"list", query.NewObject(map[string]query.Value{"stage": query.NewString("implement")})},
		{"literal.key", query.NewString("literal")},
		{"nested_fm", query.NewObject(map[string]query.Value{"stage": query.NewString("verify")})},
		{"null", query.NewNull()},
		{"object", query.NewObject(map[string]query.Value{"stage": query.NewString("plan")})},
		{"text", query.NewString("hello")},
	}
	rows := rowsFrontmatterValues(doc)
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d", len(rows), len(want))
	}
	for i, expected := range want {
		row := query.Row{
			"file":  query.NewString(doc.Path),
			"key":   query.NewString(expected.key),
			"value": expected.value,
		}
		if !reflect.DeepEqual(rows[i], row) {
			t.Errorf("row %d = %#v, want %#v", i, rows[i], row)
		}
	}
}

func TestNormalizeFloatingPointFields(t *testing.T) {
	for _, value := range []any{float32(1.5), float64(1.5)} {
		if got := normalizeFieldValue(value); got.Kind != query.ValueFloat || got.Data != float64(1.5) {
			t.Errorf("%T: %#v", value, got)
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := normalizeFieldValue(value); got.Kind != query.ValueString {
			t.Errorf("nonfinite %v must remain JSON-serializable, got %#v", value, got)
		}
	}
}

func TestFrontmatterValuesEmpty(t *testing.T) {
	for _, fm := range []model.FrontMatter{nil, {}, {"empty": []interface{}{}}} {
		rows := rowsFrontmatterValues(&model.Document{Path: "empty.md", FrontMatter: fm})
		if len(rows) != 0 {
			t.Errorf("frontmatter %#v produced %d rows, want 0", fm, len(rows))
		}
	}
}
