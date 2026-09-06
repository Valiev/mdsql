package query

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestEveryFunctionSignature(t *testing.T) {
	ctx := &EvalContext{Today: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), Now: time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC), AfterHeading: func(Row, string, int) bool { return true }}
	row := Row{"start": NewInt(1), "end": NewInt(5), "n": NewInt(3), "path": NewString("Root > Child"), "tags": NewList([]Value{NewString("work/docs")})}
	cases := []struct {
		name string
		args string
		bad  []string
		want Value
	}{
		{"at", "3", []string{"", "1, 2"}, NewBool(true)},
		{"after_heading", "'Log'", []string{"", "'Log', 2, 3"}, NewBool(true)},
		{"under", "'Root'", []string{"", "'Root', 'Child'"}, NewBool(true)},
		{"under_like", "'%oot%'", []string{"", "'Root', 'Child'"}, NewBool(true)},
		{"under_contains", "'oot'", []string{"", "'Root', 'Child'"}, NewBool(true)},
		{"has_tag", "'work'", []string{"", "'work', 'docs'"}, NewBool(true)},
		{"lower", "'ABC'", []string{"", "'A', 'B'"}, NewString("abc")},
		{"upper", "'abc'", []string{"", "'a', 'b'"}, NewString("ABC")},
		{"trim", "' x '", []string{"", "'x', 'y'"}, NewString("x")},
		{"len", "'abc'", []string{"", "'x', 'y'"}, NewInt(3)},
		{"lines", "'a\nb'", []string{"", "'x', true, false"}, NewInt(2)},
		{"head", "'a\nb', 1", []string{"", "'a'", "'a', 1, true, false"}, NewString("a")},
		{"tail", "'a\nb', 1", []string{"", "'a'", "'a', 1, true, false"}, NewString("b")},
		{"contains", "'abc', 'b'", []string{"", "'a'", "'a', 'b', 'c'"}, NewBool(true)},
		{"icontains", "'AbC', 'b'", []string{"", "'a'", "'a', 'b', 'c'"}, NewBool(true)},
		{"coalesce", "NULL, NULL, 7", []string{"", "NULL"}, NewInt(7)},
		{"round", "7, 1", []string{"", "1, 2, 3"}, NewFloat(7)},
		{"floor", "7", []string{"", "1, 2"}, NewFloat(7)},
		{"ceil", "7", []string{"", "1, 2"}, NewFloat(7)},
		{"date", "'2026-09-03'", []string{"", "'2026-09-03', '2026-09-04'"}, NewDate(ctx.Today)},
		{"basename", "'/a/b'", []string{"", "'a', 'b'"}, NewString("b")},
		{"dirname", "'/a/b'", []string{"", "'a', 'b'"}, NewString("/a")},
		{"today", "", []string{"1", "1, 2"}, NewDate(ctx.Today)},
		{"now", "", []string{"1", "1, 2"}, NewDateTime(ctx.Now)},
	}
	if len(cases) != len(functionRegistry) {
		t.Fatal("function signature cases do not cover the registry")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse("SELECT " + tc.name + "(" + tc.args + ") FROM headings")
			if err != nil {
				t.Fatal(err)
			}
			value, err := Eval(q.Select[0].Expr, row, ctx)
			if err != nil || !reflect.DeepEqual(value, tc.want) {
				t.Fatalf("value=%v want=%v error=%v", value, tc.want, err)
			}
			for _, args := range tc.bad {
				_, err := Parse("SELECT " + tc.name + "(" + args + ") FROM headings")
				if err == nil || !strings.HasPrefix(err.Error(), "at byte offset 7:") || !strings.Contains(err.Error(), "signature: "+functionRegistry[tc.name].signature) {
					t.Errorf("arguments=%q error=%v", args, err)
				}
			}
		})
	}
}

func TestFunctionArgumentTypeDiagnostics(t *testing.T) {
	row := Row{"obj": NewObject(map[string]Value{}), "list": NewList([]Value{})}
	for _, expression := range []string{
		"at('3')", "after_heading(1)", "under(1)", "under_like(1)", "under_contains(1)", "has_tag(1)",
		"lower(1)", "upper(true)", "trim(obj)", "len(true)", "lines(list)",
		"lines('x', 1)", "head('x', 1, 1)", "tail('x', 1, 'true')", "head(1, 2)", "head('x', '2')", "tail(1, 2)", "tail('x', true)",
		"contains(1, 'x')", "contains('x', 1)", "contains(list, list)", "contains(list, obj)",
		"icontains(1, 'x')", "icontains('x', true)", "date(1)", "basename(1)", "dirname(1)",
		"round('1')", "round(1, true)", "floor('1')", "ceil(true)",
	} {
		t.Run(expression, func(t *testing.T) {
			q, err := Parse("SELECT " + expression + " FROM headings")
			if err != nil {
				t.Fatal(err)
			}
			for _, strict := range []bool{false, true} {
				var warnings []string
				ctx := &EvalContext{Report: func(message string) error {
					warnings = append(warnings, message)
					if strict {
						return fmt.Errorf("%s", message)
					}
					return nil
				}}
				value, err := Eval(q.Select[0].Expr, row, ctx)
				if !value.IsNull() || (err != nil) != strict || len(warnings) != 1 || !strings.Contains(warnings[0], "expected") || !strings.Contains(warnings[0], "signature:") {
					t.Fatalf("strict=%v value=%v warnings=%v error=%v", strict, value, warnings, err)
				}
			}
		})
	}
}

func TestRoundingFunctions(t *testing.T) {
	row := Row{"value": NewFloat(12.56)}
	for _, tc := range []struct {
		expression string
		want       float64
	}{
		{"round(value)", 13},
		{"round(value, 1)", 12.6},
		{"round(value, -1)", 10},
		{"floor(-value)", -13},
		{"ceil(-value)", -12},
	} {
		q, err := Parse("SELECT " + tc.expression + " FROM files")
		if err != nil {
			t.Fatal(err)
		}
		got, err := Eval(q.Select[0].Expr, row, nil)
		if err != nil || got.Kind != ValueFloat || got.Data.(float64) != tc.want {
			t.Errorf("%s: value=%#v want=%v error=%v", tc.expression, got, tc.want, err)
		}
	}
}

func TestFunctionsAcceptNullAndSupportedOverloads(t *testing.T) {
	ctx := &EvalContext{Report: func(message string) error { return fmt.Errorf("unexpected warning: %s", message) }}
	for name, spec := range functionRegistry {
		args := make([]string, spec.minArgs)
		for i := range args {
			args[i] = "NULL"
		}
		q, err := Parse("SELECT " + name + "(" + strings.Join(args, ", ") + ") FROM headings")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Eval(q.Select[0].Expr, Row{}, ctx); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	row := Row{"list": NewList([]Value{NewNull(), NewInt(1)}), "obj": NewObject(map[string]Value{"key": NewString("value")})}
	for _, tc := range []struct {
		expression string
		want       Value
	}{
		{"contains(list, NULL)", NewBool(true)},
		{"contains(list, 1)", NewBool(true)},
		{"contains(list, '1')", NewBool(false)},
		{"contains(NULL, obj)", NewNull()},
		{"len(list)", NewInt(2)},
		{"round(1, NULL)", NewNull()},
		{"coalesce(NULL, obj)", row["obj"]},
	} {
		q, err := Parse("SELECT " + tc.expression + " FROM headings")
		if err != nil {
			t.Fatal(err)
		}
		value, err := Eval(q.Select[0].Expr, row, ctx)
		if err != nil || !reflect.DeepEqual(value, tc.want) {
			t.Errorf("%s: value=%v want=%v error=%v", tc.expression, value, tc.want, err)
		}
	}
}

func TestCountKeepsImplicitNameSeparateFromExplicitAlias(t *testing.T) {
	for _, tc := range []struct{ sql, alias, key string }{
		{"SELECT COUNT(*) FROM headings", "", "count"},
		{"SELECT COUNT(*) AS total FROM headings", "total", "total"},
	} {
		q, err := Parse(tc.sql)
		if err != nil || q.Select[0].Alias != tc.alias || q.SelectKeys()[0] != tc.key {
			t.Fatalf("query=%#v error=%v", q, err)
		}
	}
	for _, call := range []string{"COUNT()", "COUNT(*, 1)"} {
		_, err := Parse("SELECT " + call + " FROM headings")
		if err == nil || !strings.Contains(err.Error(), "COUNT(*)") || !strings.Contains(err.Error(), "at byte offset") {
			t.Errorf("call=%s error=%v", call, err)
		}
	}
}
