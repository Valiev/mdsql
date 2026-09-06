package query

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestArithmeticPrecedenceAndNumericTypes(t *testing.T) {
	row := Row{"fraction": NewFloat(1.5)}
	for _, tc := range []struct {
		expression string
		want       Value
	}{
		{"2 + 3 * 4", NewInt(14)},
		{"(2 + 3) * 4", NewInt(20)},
		{"20 / 4 + 1", NewFloat(6)},
		{"20 / (3 + 2)", NewFloat(4)},
		{"20 % 6 * 2", NewInt(4)},
		{"-2 * 3", NewInt(-6)},
		{"+2 * -3", NewInt(-6)},
		{"fraction * 2", NewFloat(3)},
		{"5 % fraction", NewFloat(.5)},
	} {
		q, err := Parse("SELECT " + tc.expression + " FROM files")
		if err != nil {
			t.Fatalf("%s: %v", tc.expression, err)
		}
		got, err := Eval(q.Select[0].Expr, row, nil)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: value=%#v want=%#v error=%v", tc.expression, got, tc.want, err)
		}
	}
}

func TestArithmeticProblemsReportAndReturnNull(t *testing.T) {
	for _, tc := range []struct {
		expression string
		row        Row
		problem    string
	}{
		{"value / zero", Row{"value": NewInt(1), "zero": NewInt(0)}, "division by zero"},
		{"value % zero", Row{"value": NewFloat(1), "zero": NewFloat(0)}, "modulo by zero"},
		{"value * value", Row{"value": NewInt(math.MaxInt64)}, "numeric overflow in *"},
		{"value * value", Row{"value": NewFloat(1e308)}, "numeric overflow in *"},
		{"value + one", Row{"value": NewInt(math.MaxInt64), "one": NewInt(1)}, "numeric overflow in +"},
		{"value - one", Row{"value": NewInt(math.MinInt64), "one": NewInt(1)}, "numeric overflow in -"},
		{"-value", Row{"value": NewInt(math.MinInt64)}, "numeric overflow in unary -"},
	} {
		q, err := Parse("SELECT " + tc.expression + " FROM files")
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
			value, err := Eval(q.Select[0].Expr, tc.row, ctx)
			if !value.IsNull() || (err != nil) != strict || len(warnings) != 1 || !strings.Contains(warnings[0], tc.problem) {
				t.Errorf("%s strict=%v: value=%#v warnings=%v error=%v", tc.expression, strict, value, warnings, err)
			}
		}
	}
}

func TestArithmeticNullAndIncompatibleValuesStayNull(t *testing.T) {
	for _, expression := range []string{"missing * 2", "missing / 2", "missing % 2", "text * 2"} {
		q, err := Parse("SELECT " + expression + " FROM files")
		if err != nil {
			t.Fatal(err)
		}
		value, err := Eval(q.Select[0].Expr, Row{"missing": NewNull(), "text": NewString("x")}, &EvalContext{Report: func(message string) error {
			return fmt.Errorf("unexpected warning: %s", message)
		}})
		if err != nil || !value.IsNull() {
			t.Errorf("%s: value=%#v error=%v", expression, value, err)
		}
	}
}
