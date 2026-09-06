package engine_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func TestAfterHeadingUsesFirstExactHeadingAcrossLaterSections(t *testing.T) {
	source := "# Before\n- old\n## Лог\n- first\n### Лог\n- child\n## Later\n- later\n"
	doc, err := parser.ParseFile("synthetic.md", source, int64(len(source)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	q, err := query.Parse("SELECT n, text FROM lines WHERE after_heading('лог') AND text LIKE '- %' ORDER BY n DESC LIMIT 2")
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Execute(q, []*parser.Document{doc}, engine.Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []query.Row{
		{"n": query.NewInt(8), "text": query.NewString("- later")},
		{"n": query.NewInt(6), "text": query.NewString("- child")},
	}
	if !reflect.DeepEqual(result.Rows, want) {
		t.Fatalf("rows=%#v want=%#v", result.Rows, want)
	}

	q, err = query.Parse("SELECT n FROM lines WHERE after_heading('Лог', 3) ORDER BY n")
	if err != nil {
		t.Fatal(err)
	}
	result, err = engine.Execute(q, []*parser.Document{doc}, engine.Options{Strict: true})
	if err != nil || len(result.Rows) != 3 || result.Rows[0]["n"].Data != int64(6) {
		t.Fatalf("level result=%#v error=%v", result, err)
	}
}

func TestAfterHeadingIsFalseOutsideLinesAndKeepsDocumentsSeparate(t *testing.T) {
	first, err := parser.ParseFile("same.md", "# Log\nvalue\n", 12, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := parser.ParseFile("same.md", "value\n", 6, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		sql  string
		rows int
	}{
		{"SELECT file FROM lines WHERE after_heading('Log')", 1},
		{"SELECT file FROM files WHERE after_heading('Log')", 0},
	} {
		q, parseErr := query.Parse(test.sql)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		result, executeErr := engine.Execute(q, []*parser.Document{first, second}, engine.Options{Strict: true})
		if executeErr != nil || len(result.Rows) != test.rows {
			t.Fatalf("%s: result=%#v error=%v", test.sql, result, executeErr)
		}
	}
}

func TestAfterHeadingValidatesLevel(t *testing.T) {
	doc, err := parser.ParseFile("synthetic.md", "# Log\nvalue\n", 12, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	q, err := query.Parse("SELECT n FROM lines WHERE after_heading('Log', 0)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Execute(q, []*parser.Document{doc}, engine.Options{Strict: true}); err == nil {
		t.Fatal("invalid heading level was accepted")
	}
}
