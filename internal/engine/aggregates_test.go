package engine

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func aggregateDocuments(values ...interface{}) []*model.Document {
	docs := make([]*model.Document, len(values))
	for i, value := range values {
		docs[i] = &model.Document{Path: fmt.Sprintf("synthetic-%d.md", i), FrontMatter: model.FrontMatter{"value": value}}
	}
	return docs
}

func executeAggregateSQL(t *testing.T, sql string, docs []*model.Document, strict bool) (*ExecuteResult, error) {
	t.Helper()
	q, err := query.Parse(sql)
	if err != nil {
		return nil, err
	}
	return Execute(q, docs, Options{Strict: strict})
}

func TestNumericAggregatesAndNulls(t *testing.T) {
	for _, tc := range []struct {
		name   string
		values []interface{}
		want   query.Row
	}{
		{"integers", []interface{}{1, 2, nil}, query.Row{"count": query.NewInt(3), "present": query.NewInt(2), "sum": query.NewInt(3), "min": query.NewInt(1), "max": query.NewInt(2), "avg": query.NewFloat(1.5)}},
		{"mixed", []interface{}{1, 2.5, nil}, query.Row{"count": query.NewInt(3), "present": query.NewInt(2), "sum": query.NewFloat(3.5), "min": query.NewInt(1), "max": query.NewFloat(2.5), "avg": query.NewFloat(1.75)}},
		{"nulls", []interface{}{nil, nil}, query.Row{"count": query.NewInt(2), "present": query.NewInt(0), "sum": query.NewNull(), "min": query.NewNull(), "max": query.NewNull(), "avg": query.NewNull()}},
		{"empty", nil, query.Row{"count": query.NewInt(0), "present": query.NewInt(0), "sum": query.NewNull(), "min": query.NewNull(), "max": query.NewNull(), "avg": query.NewNull()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := executeAggregateSQL(t, "SELECT count(*), count(fm.value) AS present, sum(fm.value), min(fm.value), max(fm.value), avg(fm.value) FROM frontmatter", aggregateDocuments(tc.values...), true)
			if err != nil || len(result.Rows) != 1 || !reflect.DeepEqual(result.Rows[0], tc.want) {
				t.Fatalf("result=%#v want=%v error=%v", result, tc.want, err)
			}
		})
	}
}

func TestAggregateGroupsAliasesAndExpressionWrappers(t *testing.T) {
	docs := aggregateDocuments(2, 4, nil, 1.5, 2.5, nil)
	for i, group := range []string{"a", "a", "a", "b", "b", "c"} {
		docs[i].FrontMatter["group"] = group
	}
	q, err := query.Parse("SELECT fm.group AS category, sum(fm.value) AS total, avg(fm.value) + 1 AS mean, count(fm.value) AS present, count(*) AS rows FROM frontmatter GROUP BY category ORDER BY total DESC, category")
	if err != nil {
		t.Fatal(err)
	}
	before := q.Select[1].Expr
	result, err := Execute(q, docs, Options{Strict: true})
	if err != nil || len(result.Rows) != 3 || q.Select[1].Expr != before {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	// Existing DESC semantics put NULL first.
	if result.Rows[0]["category"].String() != "c" || result.Rows[1]["category"].String() != "a" || result.Rows[1]["mean"].Data != float64(4) || result.Rows[2]["total"].Data != float64(4) || result.Rows[0]["present"].Data != int64(0) {
		t.Fatalf("groups=%v", result.Rows)
	}
	result, err = executeAggregateSQL(t, "SELECT fm.group, max(fm.value), count(*) FROM frontmatter GROUP BY fm.group ORDER BY sum(fm.value) DESC LIMIT 1 OFFSET 1", docs, true)
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["fm.group"].String() != "a" {
		t.Fatalf("aggregate ORDER BY result=%#v error=%v", result, err)
	}
	result, err = executeAggregateSQL(t, "SELECT fm.group, count(*) FROM frontmatter GROUP BY fm.group", nil, true)
	if err != nil || len(result.Rows) != 0 || !reflect.DeepEqual(result.Columns, []string{"fm.group", "count"}) {
		t.Fatalf("empty groups result=%#v error=%v", result, err)
	}
}

func TestMinMaxStringsAndDates(t *testing.T) {
	for _, tc := range []struct {
		values    []interface{}
		low, high query.Value
	}{
		{[]interface{}{"zebra", nil, "alpha"}, query.NewString("alpha"), query.NewString("zebra")},
		{[]interface{}{"2026-09-03", nil, "2026-09-01"}, query.NewDate(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)), query.NewDate(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))},
		{[]interface{}{time.Unix(20, 0).UTC(), nil, time.Unix(10, 0).UTC()}, query.NewDateTime(time.Unix(10, 0).UTC()), query.NewDateTime(time.Unix(20, 0).UTC())},
	} {
		result, err := executeAggregateSQL(t, "SELECT min(fm.value), max(fm.value) FROM frontmatter", aggregateDocuments(tc.values...), true)
		if err != nil || !reflect.DeepEqual(result.Rows, []query.Row{{"min": tc.low, "max": tc.high}}) {
			t.Fatalf("result=%#v error=%v", result, err)
		}
	}
	result, err := executeAggregateSQL(t, "SELECT upper(min(fm.value)) AS first FROM frontmatter", aggregateDocuments("zebra", "alpha"), true)
	if err != nil || result.Rows[0]["first"].String() != "ALPHA" {
		t.Fatalf("scalar wrapper result=%#v error=%v", result, err)
	}
}

func TestAggregateValidationWarningsAndStrict(t *testing.T) {
	for _, tc := range []struct {
		sql    string
		values []interface{}
	}{
		{"SELECT sum(fm.value) AS result FROM frontmatter", []interface{}{1, "bad"}},
		{"SELECT avg(fm.value) AS result FROM frontmatter", []interface{}{1, true}},
		{"SELECT min(fm.value) AS result FROM frontmatter", []interface{}{1, "bad"}},
		{"SELECT max(fm.value) AS result FROM frontmatter", []interface{}{map[string]interface{}{"x": 1}}},
		{"SELECT sum(fm.value) AS result FROM frontmatter", []interface{}{int64(math.MaxInt64), int64(1)}},
		{"SELECT sum(fm.value) AS result FROM frontmatter", []interface{}{1e308, 1e308}},
	} {
		for _, strict := range []bool{false, true} {
			result, err := executeAggregateSQL(t, tc.sql, aggregateDocuments(tc.values...), strict)
			if strict {
				if err == nil {
					t.Errorf("strict accepted %s values=%v", tc.sql, tc.values)
				}
			} else if err != nil || len(result.Warnings) != 1 || !result.Rows[0]["result"].IsNull() {
				t.Errorf("result=%#v error=%v", result, err)
			}
		}
	}
	for _, sql := range []string{
		"SELECT file, sum(fm.value) FROM frontmatter",
		"SELECT fm.group, fm.value, sum(fm.value) FROM frontmatter GROUP BY fm.group",
		"SELECT sum(fm.value) AS total FROM frontmatter GROUP BY total",
		"SELECT sum(fm.value) FROM frontmatter ORDER BY file",
		"SELECT sum(sum(fm.value)) FROM frontmatter",
		"SELECT file FROM frontmatter WHERE avg(fm.value) > 0",
	} {
		if _, err := executeAggregateSQL(t, sql, aggregateDocuments(1, 2), true); err == nil {
			t.Errorf("accepted invalid aggregate query %s", sql)
		}
	}
}

func TestAggregateProjectionNamesAndLargeMixedSum(t *testing.T) {
	result, err := executeAggregateSQL(t, "SELECT count(*), count(fm.value), sum(fm.value), sum(fm.value), avg(fm.value) AS sum_2 FROM frontmatter", aggregateDocuments(1, 2), true)
	if err != nil || !reflect.DeepEqual(result.Columns, []string{"count", "count_2", "sum", "sum_3", "sum_2"}) {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	result, err = executeAggregateSQL(t, "SELECT sum(fm.value) AS result FROM frontmatter", aggregateDocuments(int64(math.MaxInt64), int64(math.MaxInt64), float64(1)), true)
	if err != nil || result.Rows[0]["result"].Kind != query.ValueFloat || len(result.Warnings) > 0 {
		t.Fatalf("mixed sum result=%#v error=%v", result, err)
	}
	result, err = executeAggregateSQL(t, "SELECT count(fm.value) AS result FROM frontmatter", aggregateDocuments(true, "text", nil, []interface{}{1}), true)
	if err != nil || result.Rows[0]["result"].Data != int64(3) {
		t.Fatalf("count result=%#v error=%v", result, err)
	}
	_, err = executeAggregateSQL(t, "SELECT sum(unknown) FROM frontmatter", nil, true)
	if err == nil || !strings.Contains(err.Error(), "unknown column") {
		t.Fatalf("missing static validation: %v", err)
	}
}
