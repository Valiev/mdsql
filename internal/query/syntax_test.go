package query

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUnicodeIdentifiers(t *testing.T) {
	q, err := Parse("SELECT title AS Заголовок, fm.данные.имя AS 名称２ FROM headings WHERE fm.данные.имя = 'Тест' ORDER BY Заголовок")
	if err != nil {
		t.Fatal(err)
	}
	if got := q.SelectKeys(); !reflect.DeepEqual(got, []string{"Заголовок", "名称２"}) {
		t.Fatalf("keys=%v", got)
	}
	row := Row{
		"title": NewString("Пример"),
		"fm": NewObject(map[string]Value{
			"данные": NewObject(map[string]Value{"имя": NewString("Тест")}),
		}),
	}
	value, err := Eval(q.Select[1].Expr, row, nil)
	if err != nil || value.String() != "Тест" {
		t.Fatalf("SELECT value=%v error=%v", value, err)
	}
	value, err = Eval(q.Where, row, nil)
	if err != nil || !truthy(value) {
		t.Fatalf("WHERE value=%v error=%v", value, err)
	}
	q, err = Parse("SELECT fm.имя, COUNT(*) AS Количество FROM frontmatter GROUP BY fm.имя")
	if err != nil || q.GroupBy != "fm.имя" || q.Select[1].Alias != "Количество" {
		t.Fatalf("GROUP BY query=%v error=%v", q, err)
	}
}

func TestContextFunctionAliases(t *testing.T) {
	q, err := Parse("SELECT today, TODAY(), now, NOW() FROM headings WHERE today() = today AND now() = now")
	if err != nil {
		t.Fatal(err)
	}
	ctx := &EvalContext{Today: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC), Now: time.Date(2026, 9, 3, 12, 34, 56, 0, time.UTC)}
	for i, item := range q.Select {
		got, err := Eval(item.Expr, Row{}, ctx)
		want := NewDate(ctx.Today)
		if i >= 2 {
			want = NewDateTime(ctx.Now)
		}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("item %d value=%v want=%v error=%v", i, got, want, err)
		}
	}
	value, err := Eval(q.Where, Row{}, ctx)
	if err != nil || !truthy(value) {
		t.Fatalf("WHERE value=%v error=%v", value, err)
	}
}

func TestParseErrorsHaveByteOffsetsAndExpectedTokens(t *testing.T) {
	for _, tc := range []struct {
		sql      string
		at       string
		expected string
	}{
		{"", "", "SELECT"},
		{"SELECT title headings", "headings", "FROM"},
		{"SELECT FROM headings", "FROM", "expression"},
		{"SELECT title AS 1 FROM headings", "1", "alias after AS"},
		{"SELECT title AS FROM headings", "FROM", "alias after AS"},
		{"SELECT COUNT(*) AS FROM headings", "FROM", "alias after AS"},
		{"SELECT title FROM WHERE title = 'x'", "WHERE", "table name"},
		{"SELECT title FROM headings WHERE ORDER BY title", "ORDER", "expression"},
		{"SELECT title AS Заголовок WHERE title = 'x'", "WHERE", "FROM"},
		{"SELECT lower(title FROM headings", "FROM", ")"},
		{"SELECT title FROM headings WHERE", "", "expression"},
		{"SELECT title FROM headings LIMIT abc", "abc", "integer after LIMIT"},
		{"SELECT title FROM headings OFFSET", "", "integer after OFFSET"},
		{"SELECT title FROM headings ORDER title", "title", "BY after ORDER"},
		{"SELECT title FROM headings GROUP title", "title", "BY after GROUP"},
		{"SELECT title FROM headings WHERE title IS 'x'", "'x'", "NULL after IS"},
		{"SELECT title FROM headings WHERE title IN ('x'", "", ") after IN values"},
		{"SELECT title AS 😀 FROM headings", "😀", "alias after AS"},
		{"SELECT @ FROM headings", "@", "expression"},
		{"SELECT 'незакрытая строка", "", "closing"},
		{"SELECT title FROM headings\n-- comment\nWHERE title =", "", "expression"},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			_, err := Parse(tc.sql)
			pos := len(tc.sql)
			if tc.at != "" {
				pos = strings.LastIndex(tc.sql, tc.at)
			}
			want := fmt.Sprintf("at byte offset %d: expected %s", pos, tc.expected)
			if err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("error=%v, want prefix=%q", err, want)
			}
		})
	}
}

func TestParsePreservesExplicitLimitZero(t *testing.T) {
	for _, tc := range []struct {
		suffix string
		limit  int
		set    bool
	}{
		{"", 0, false},
		{" LIMIT 0", 0, true},
		{" LIMIT 2 OFFSET 1", 2, true},
	} {
		q, err := Parse("SELECT title FROM headings" + tc.suffix)
		if err != nil || q.Limit != tc.limit || q.LimitSet != tc.set {
			t.Fatalf("suffix=%q query=%#v error=%v", tc.suffix, q, err)
		}
	}
}
