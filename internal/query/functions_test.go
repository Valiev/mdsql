package query

import (
	"strings"
	"testing"
)

func TestParseRejectsUnknownFunctions(t *testing.T) {
	for _, sql := range []string{
		"SELECT substr(title, 1, 3) FROM headings",
		"SELECT lower(substr(title, 1, 3)) FROM headings",
		"SELECT title FROM headings WHERE substr(title, 1, 3) = 'abc'",
		"SELECT title FROM headings WHERE false AND substr(title, 1, 3) = 'abc'",
		"SELECT title FROM headings WHERE true OR substr(title, 1, 3) = 'abc'",
		"SELECT title FROM headings WHERE title IN (substr('abc', 1, 3))",
		"SELECT title FROM headings WHERE title BETWEEN substr('abc', 1, 3) AND 'z'",
		"SELECT title FROM headings ORDER BY substr(title, 1, 3)",
	} {
		t.Run(sql, func(t *testing.T) {
			_, err := Parse(sql)
			if err == nil || !strings.Contains(err.Error(), `unknown function "substr"`) || !strings.Contains(err.Error(), "available functions: ") || !strings.Contains(err.Error(), "lower(s)") {
				t.Fatalf("Parse error = %v", err)
			}
		})
	}
}

func TestEvalRejectsUnknownFunction(t *testing.T) {
	_, err := Eval(&FuncCallExpr{Name: "regex"}, Row{}, nil)
	if err == nil || !strings.Contains(err.Error(), `unknown function "regex"`) {
		t.Fatalf("Eval error = %v", err)
	}
}

func TestKnownFunctionsRemainAvailable(t *testing.T) {
	for name, spec := range functionRegistry {
		args := make([]string, spec.minArgs)
		for i := range args {
			args[i] = "NULL"
		}
		if _, err := Parse("SELECT " + strings.ToUpper(name) + "(" + strings.Join(args, ", ") + ") FROM headings"); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	q, err := Parse("SELECT UPPER(trim(title)) FROM headings WHERE contains(title, 'Demo')")
	if err != nil {
		t.Fatal(err)
	}
	row := Row{"title": NewString(" Demo ")}
	value, err := Eval(q.Select[0].Expr, row, nil)
	if err != nil || value.Kind != ValueString || value.String() != "DEMO" {
		t.Fatalf("SELECT value=%v error=%v", value, err)
	}
	value, err = Eval(q.Where, row, nil)
	if err != nil || value.Kind != ValueBool || !truthy(value) {
		t.Fatalf("WHERE value=%v error=%v", value, err)
	}
	if q, err := Parse("SELECT COUNT(*) FROM headings"); err != nil || !q.CountAll {
		t.Fatalf("COUNT query=%v error=%v", q, err)
	}
}
