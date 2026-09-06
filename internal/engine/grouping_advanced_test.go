package engine

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func TestMultipleGroupByAndHaving(t *testing.T) {
	values := []struct {
		stage, owner string
	}{
		{"active", "a"}, {"active", "a"}, {"active", "b"},
		{"done", "a"}, {"done", "b"}, {"done", "b"},
	}
	docs := aggregateDocuments(make([]interface{}, len(values))...)
	for i, value := range values {
		docs[i].FrontMatter["stage"] = value.stage
		docs[i].FrontMatter["owner"] = value.owner
	}
	result, err := executeAggregateSQL(t, "SELECT fm.stage AS stage, fm.owner AS owner, count(*) AS n FROM frontmatter GROUP BY stage, owner HAVING n > 1 ORDER BY stage, owner", docs, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []query.Row{
		{"stage": query.NewString("active"), "owner": query.NewString("a"), "n": query.NewInt(2)},
		{"stage": query.NewString("done"), "owner": query.NewString("b"), "n": query.NewInt(2)},
	}
	if !reflect.DeepEqual(result.Rows, want) {
		t.Fatalf("rows=%#v want=%#v", result.Rows, want)
	}

	result, err = executeAggregateSQL(t, "SELECT fm.stage AS stage, count(*) AS n FROM frontmatter GROUP BY stage HAVING count(*) > 2 ORDER BY stage", docs, true)
	if err != nil || len(result.Rows) != 2 {
		t.Fatalf("direct aggregate HAVING: result=%#v error=%v", result, err)
	}
}

func TestLimitPerGroupAfterOrdering(t *testing.T) {
	documents := make([]*parser.Document, 0, 2)
	for _, name := range []string{"a.md", "b.md"} {
		source := strings.Join([]string{"one", "two", "three", "four", "five"}, "\n") + "\n"
		doc, err := parser.ParseFile(name, source, int64(len(source)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		documents = append(documents, doc)
	}
	q, err := query.Parse("SELECT file, n, text FROM lines ORDER BY file, n DESC LIMIT 2 PER file")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Execute(q, documents, Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 4 {
		t.Fatalf("rows=%#v", result.Rows)
	}
	for i, wantN := range []int64{5, 4, 5, 4} {
		if result.Rows[i]["n"].Data != wantN {
			t.Fatalf("row %d=%#v", i, result.Rows[i])
		}
	}

	result, err = Execute(q, documents, Options{Strict: true, TableLimit: 3})
	if err != nil || len(result.Rows) != 3 {
		t.Fatalf("global limit after per-group limit: result=%#v error=%v", result, err)
	}
}

func TestAdvancedGroupingValidation(t *testing.T) {
	for _, sql := range []string{
		"SELECT fm.stage, count(*) FROM frontmatter GROUP BY fm.stage, missing",
		"SELECT file AS f, count(*) AS n FROM files GROUP BY f HAVING size > 1",
		"SELECT count(*) AS n FROM files HAVING file = 'a.md'",
		"SELECT count(*) AS n FROM files LIMIT 1 PER n",
	} {
		q, err := query.Parse(sql)
		if err != nil {
			continue
		}
		if _, err := Execute(q, nil, Options{Strict: true}); err == nil {
			t.Fatalf("expected error for %q", sql)
		}
	}
}
