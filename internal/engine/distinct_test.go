package engine

import (
	"reflect"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func TestDistinctValuesDoNotCollide(t *testing.T) {
	stamp := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name string
		a, b query.Value
	}{
		{"null-string", query.NewNull(), query.NewString("")},
		{"int-string", query.NewInt(1), query.NewString("1")},
		{"bool-string", query.NewBool(true), query.NewString("true")},
		{"date-string", query.NewDate(stamp), query.NewString("2026-09-03")},
		{"date-timestamp", query.NewDate(stamp), query.NewDateTime(stamp)},
		{"timestamp-nanoseconds", query.NewDateTime(stamp), query.NewDateTime(stamp.Add(time.Nanosecond))},
		{"list-delimiters", query.NewList([]query.Value{query.NewString("a,b"), query.NewString("c")}), query.NewList([]query.Value{query.NewString("a"), query.NewString("b,c")})},
		{"list-types", query.NewList([]query.Value{query.NewInt(1)}), query.NewList([]query.Value{query.NewString("1")})},
		{"objects", query.NewObject(map[string]query.Value{"x": query.NewInt(1)}), query.NewObject(map[string]query.Value{"x": query.NewInt(2)})},
		{"nested-internal-key", query.NewObject(map[string]query.Value{"__x": query.NewInt(1)}), query.NewObject(map[string]query.Value{"__x": query.NewInt(2)})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := []query.Row{{"value": tc.a}, {"value": tc.b}, {"value": tc.a}}
			got := uniqueRows(rows)
			if !reflect.DeepEqual(got, rows[:2]) {
				t.Fatalf("distinct result=%#v want first two input rows", got)
			}
		})
	}
}

func TestDistinctCanonicalObjectsAndTimes(t *testing.T) {
	stamp := time.Date(2026, 9, 3, 0, 0, 0, 123, time.UTC)
	a := query.NewObject(map[string]query.Value{"a": query.NewList([]query.Value{query.NewInt(1)}), "b": query.NewDateTime(stamp)})
	b := query.NewObject(map[string]query.Value{"b": query.NewDateTime(stamp.In(time.FixedZone("offset", 3*3600))), "a": query.NewList([]query.Value{query.NewInt(1)})})
	rows := []query.Row{{"value": a, "__source": query.NewInt(1)}, {"__source": query.NewInt(2), "value": b}}
	if got := uniqueRows(rows); !reflect.DeepEqual(got, rows[:1]) {
		t.Fatalf("same values did not deduplicate: %#v", got)
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Fatal("DISTINCT mutated source rows")
	}
}

func TestDistinctSQLKeepsDelimiterValues(t *testing.T) {
	docs := []*model.Document{
		{FrontMatter: model.FrontMatter{"x": "a", "y": "b|fm.y=c"}},
		{FrontMatter: model.FrontMatter{"x": "a|fm.y=b", "y": "c"}},
		{FrontMatter: model.FrontMatter{"x": "a", "y": "b|fm.y=c"}},
	}
	q, err := query.Parse("SELECT DISTINCT fm.x, fm.y FROM frontmatter")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Execute(q, docs, Options{})
	if err != nil || len(res.Rows) != 2 {
		t.Fatalf("rows=%v error=%v", res.Rows, err)
	}
	if res.Rows[0]["fm.x"].String() != "a" || res.Rows[1]["fm.x"].String() != "a|fm.y=b" {
		t.Fatal("input order not retained")
	}
}
