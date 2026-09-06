package query

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDateComparisonsWithISOStrings(t *testing.T) {
	date := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		value Value
		equal string
		low   string
		high  string
	}{
		{"date", NewDate(date), "2026-09-03", "2026-09-02", "2026-09-04"},
		{"datetime", NewDateTime(date), "2026-09-03T03:00:00+03:00", "2026-09-02T23:59:59Z", "2026-09-03T00:00:01Z"},
		{"datetime-date-only", NewDateTime(date), "2026-09-03", "2026-09-02", "2026-09-04"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := Row{"value": tc.value}
			for _, predicate := range []string{
				fmt.Sprintf("value = '%s'", tc.equal),
				fmt.Sprintf("'%s' = value", tc.equal),
				fmt.Sprintf("value < '%s'", tc.high),
				fmt.Sprintf("value >= '%s'", tc.low),
				fmt.Sprintf("value <= '%s'", tc.equal),
				fmt.Sprintf("value > '%s'", tc.low),
				fmt.Sprintf("value != '%s'", tc.high),
				fmt.Sprintf("value BETWEEN '%s' AND '%s'", tc.low, tc.high),
				fmt.Sprintf("value IN ('%s', '%s')", tc.high, tc.equal),
				fmt.Sprintf("value NOT IN ('%s', '%s')", tc.low, tc.high),
			} {
				q, err := Parse("SELECT value FROM headings WHERE " + predicate)
				if err != nil {
					t.Fatal(err)
				}
				ctx := &EvalContext{Report: func(message string) error { return fmt.Errorf("unexpected warning: %s", message) }}
				got, err := Eval(q.Where, row, ctx)
				if err != nil || got.Kind != ValueBool || !truthy(got) {
					t.Errorf("%s: value=%v error=%v", predicate, got, err)
				}
				if !reflect.DeepEqual(row["value"], tc.value) {
					t.Fatalf("comparison mutated value: %v", row["value"])
				}
			}
		})
	}
}

func TestInvalidDateComparisonsReportInAllOperators(t *testing.T) {
	for _, value := range []Value{NewDate(time.Time{}), NewDateTime(time.Time{})} {
		for _, predicate := range []string{
			"value = 'invalid'", "value < 'invalid'", "value >= 'invalid'",
			"value BETWEEN 'invalid' AND '2026-09-03'",
			"value BETWEEN '2026-09-02' AND 'invalid'",
			"value IN ('invalid')", "value NOT IN ('invalid')",
			"value IN (1)",
		} {
			q, err := Parse("SELECT value FROM headings WHERE " + predicate)
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
				_, err := Eval(q.Where, Row{"value": value}, ctx)
				if (err != nil) != strict || len(warnings) != 1 || !strings.Contains(warnings[0], "incompatible comparison") {
					t.Errorf("%s %s strict=%v warnings=%v error=%v", value.Kind, predicate, strict, warnings, err)
				}
			}
		}
	}
}

func TestINChecksInvalidMembersAfterAMatch(t *testing.T) {
	q, err := Parse("SELECT value FROM headings WHERE value IN ('2026-09-03', 'invalid')")
	if err != nil {
		t.Fatal(err)
	}
	date := NewDate(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	for _, strict := range []bool{false, true} {
		var warnings []string
		ctx := &EvalContext{Report: func(message string) error {
			warnings = append(warnings, message)
			if strict {
				return fmt.Errorf("%s", message)
			}
			return nil
		}}
		value, err := Eval(q.Where, Row{"value": date}, ctx)
		if (err != nil) != strict || (!strict && !truthy(value)) || len(warnings) != 1 || !strings.Contains(warnings[0], "IN item 2") {
			t.Fatalf("strict=%v value=%v warnings=%v error=%v", strict, value, warnings, err)
		}
	}
}

func TestDateCoercionPreservesStringComparisonsAndListValues(t *testing.T) {
	for _, predicate := range []string{
		"'2026-09-03' != '2026-09-03T00:00:00Z'",
		"'2026-09-03' NOT IN ('2026-09-03T00:00:00Z')",
		"'2026-09-03' = 2026-09-03",
		"'2026-09-03' IN (2026-09-03)",
		"'2026-09-03' BETWEEN 2026-09-02 AND 2026-09-04",
	} {
		q, err := Parse("SELECT value FROM headings WHERE " + predicate)
		if err != nil {
			t.Fatal(err)
		}
		value, err := Eval(q.Where, Row{}, &EvalContext{Report: func(message string) error { return fmt.Errorf("%s", message) }})
		if err != nil || !truthy(value) {
			t.Errorf("%s: value=%v error=%v", predicate, value, err)
		}
	}
	q, err := Parse("SELECT value FROM headings WHERE value IN ('2026-09-03')")
	if err != nil {
		t.Fatal(err)
	}
	date := NewDate(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if _, err := Eval(q.Where, Row{"value": date}, nil); err != nil {
		t.Fatal(err)
	}
	values := q.Where.(*BinaryExpr).Right.(*LiteralExpr).Value.Data.([]Value)
	if len(values) != 1 || values[0].Kind != ValueString {
		t.Fatalf("IN list was mutated: %v", values)
	}
}
