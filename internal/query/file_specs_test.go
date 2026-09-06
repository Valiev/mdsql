package query

import (
	"reflect"
	"strings"
	"testing"
)

func TestParenthesizedFileSpecsMatchPlainLists(t *testing.T) {
	for _, sources := range []string{"'a.md'", "'a.md', 'b.md'", `'notes/a (1),b.md', "quote's.md"`, `'dir\\file.md', 'Имя файла.md'`, `'tab\tfile.md', 'line\nfile.md'`} {
		plain, err := Parse("SELECT title FROM headings IN " + sources + " WHERE level = 1")
		if err != nil {
			t.Fatal(err)
		}
		wrapped, err := Parse("SELECT title FROM headings IN (" + sources + ") WHERE level = 1")
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(wrapped.In, plain.In) || wrapped.Table != plain.Table {
			t.Fatalf("sources=%q wrapped=%v plain=%v", sources, wrapped.In, plain.In)
		}
		again, err := Parse(wrapped.String())
		if err != nil || !reflect.DeepEqual(again.In, wrapped.In) {
			t.Fatalf("normalized=%q reparsed=%#v error=%v", wrapped.String(), again, err)
		}
		if !strings.HasSuffix(wrapped.String(), " WHERE level = 1") || strings.Contains(wrapped.String(), " IN (") {
			t.Fatalf("normalized=%q", wrapped.String())
		}
	}
}

func TestFileSpecNormalizationPreservesOtherClauses(t *testing.T) {
	sql := "SELECT title AS Заголовок FROM headings IN (\n'a.md', -- source comment\n'b.md') WHERE title IN ('A', 'B') -- predicate comment\nORDER BY Заголовок"
	q, err := Parse(sql)
	if err != nil {
		t.Fatal(err)
	}
	want := "SELECT title AS Заголовок FROM headings IN 'a.md', 'b.md' WHERE title IN ('A', 'B') -- predicate comment\nORDER BY Заголовок"
	if q.String() != want {
		t.Fatalf("normalized=%q want=%q", q.String(), want)
	}
	plain := "SELECT title FROM headings IN docs/**/*.md, 'other.md' WHERE level = 1"
	q, err = Parse(plain)
	if err != nil || q.String() != plain || !reflect.DeepEqual(q.In, []string{"docs/**/*.md", "other.md"}) {
		t.Fatalf("unparenthesized=%#v error=%v", q, err)
	}
}

func TestMalformedSourceListsAreParseErrors(t *testing.T) {
	for _, sources := range []string{
		"()", "('a.md'", "('a.md',)", "('a.md' 'b.md')", "(('a.md'))",
		"(a.md)", "('')", "('a.md', '')", "('a.md', WHERE)", "''",
	} {
		_, err := Parse("SELECT title FROM headings IN " + sources)
		if err == nil || !strings.Contains(err.Error(), "at byte offset") || !strings.Contains(err.Error(), "file specification") {
			t.Errorf("sources=%q error=%v", sources, err)
		}
	}
}
