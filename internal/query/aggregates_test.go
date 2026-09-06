package query

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestParseAggregateExpressions(t *testing.T) {
	q, err := Parse("SELECT COUNT(*), count(size), sum(size), min(size), max(size), avg(size) AS mean FROM files ORDER BY mean")
	if err != nil {
		t.Fatal(err)
	}
	if !q.CountAll || !reflect.DeepEqual(q.SelectKeys(), []string{"count", "count", "sum", "min", "max", "mean"}) {
		t.Fatalf("query=%#v", q)
	}
	for _, item := range q.Select {
		if _, ok := item.Expr.(*AggregateExpr); !ok {
			t.Fatalf("expression=%T", item.Expr)
		}
	}
	for _, sql := range []string{
		"SELECT sum() FROM files", "SELECT avg(size, size) FROM files", "SELECT min(*) FROM files",
		"SELECT sum(avg(size)) FROM files", "SELECT max(lower(min(name))) FROM files",
		"SELECT file FROM files WHERE sum(size) > 0", "SELECT file FROM files WHERE false AND count(*) = 1",
	} {
		if _, err := Parse(sql); err == nil {
			t.Errorf("accepted %s", sql)
		}
	}
}

func TestFloatComparisonAndArithmetic(t *testing.T) {
	row := Row{"a": NewFloat(1.5), "b": NewInt(2), "large": NewInt(9007199254740993), "rounded": NewFloat(9007199254740992)}
	for _, predicate := range []string{"a < b", "b >= a", "a + 1 > b", "b - a < 1", "-a < 0", "large > rounded", "rounded < large", "a BETWEEN 1 AND 2", "b IN (2)"} {
		q, err := Parse("SELECT a FROM files WHERE " + predicate)
		if err != nil {
			t.Fatal(err)
		}
		value, err := Eval(q.Where, row, &EvalContext{Report: func(message string) error { return fmt.Errorf("%s", message) }})
		if err != nil || !truthy(value) {
			t.Errorf("%s: value=%v error=%v", predicate, value, err)
		}
	}
	rows := []Row{{"v": NewFloat(2.5)}, {"v": NewInt(1)}, {"v": NewFloat(1.5)}, {"v": NewInt(2)}}
	if err := Order(rows, []OrderItem{{Expr: &ColumnExpr{Name: "v"}}}, nil); err != nil {
		t.Fatal(err)
	}
	for i, want := range []float64{1, 1.5, 2, 2.5} {
		if Number(rows[i]["v"]) != want {
			t.Fatalf("sorted rows=%v", rows)
		}
	}
}

func TestFloatOverflowReportsBeforeComparison(t *testing.T) {
	for _, predicate := range []string{"large + large > 0", "negative - large < 0"} {
		q, err := Parse("SELECT large FROM files WHERE " + predicate)
		if err != nil {
			t.Fatal(err)
		}
		for _, strict := range []bool{false, true} {
			warnings := []string{}
			ctx := &EvalContext{Report: func(message string) error {
				warnings = append(warnings, message)
				if strict {
					return fmt.Errorf("%s", message)
				}
				return nil
			}}
			value, err := Eval(q.Where, Row{"large": NewFloat(1e308), "negative": NewFloat(-1e308)}, ctx)
			if (err != nil) != strict || len(warnings) != 1 || !strings.Contains(warnings[0], "numeric overflow") || truthy(value) {
				t.Fatalf("strict=%v value=%v warnings=%v error=%v", strict, value, warnings, err)
			}
		}
	}
	if Compare(NewFloat(math.Inf(1)), NewInt(0)) <= 0 || Compare(NewFloat(math.Inf(-1)), NewInt(0)) >= 0 {
		t.Fatal("nonfinite comparison is not ordered")
	}
	_ = Compare(NewFloat(math.NaN()), NewInt(0))
}
