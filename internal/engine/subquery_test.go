package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func subqueryDocuments(t *testing.T) []*model.Document {
	t.Helper()
	sources := []struct {
		name, markdown string
	}{
		{"active-a.md", "---\nstatus: active\n---\n# Work\n- [ ] first\n"},
		{"active-b.md", "---\nstatus: active\n---\n# Other\n- [ ] second\n"},
		{"paused.md", "---\nstatus: paused\n---\n# Work\n- [ ] third\n"},
	}
	docs := make([]*model.Document, 0, len(sources))
	for _, source := range sources {
		doc, err := parser.ParseFile(source.name, source.markdown, int64(len(source.markdown)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, doc)
	}
	return docs
}

func TestExecuteINSubquery(t *testing.T) {
	docs := subqueryDocuments(t)
	tests := []struct {
		sql  string
		want []string
	}{
		{
			"SELECT text FROM tasks WHERE file IN (SELECT file FROM frontmatter WHERE fm.status = 'active') ORDER BY text",
			[]string{"first", "second"},
		},
		{
			"SELECT text FROM tasks WHERE file NOT IN (SELECT file FROM frontmatter WHERE fm.status = 'active')",
			[]string{"third"},
		},
		{
			"SELECT text FROM tasks WHERE section IN (SELECT title FROM headings WHERE level = 1) ORDER BY text",
			[]string{"first", "second", "third"},
		},
	}
	for _, tc := range tests {
		q, err := query.Parse(tc.sql)
		if err != nil {
			t.Fatal(err)
		}
		result, err := Execute(q, docs, Options{Strict: true})
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, 0, len(result.Rows))
		for _, row := range result.Rows {
			got = append(got, row["text"].String())
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: rows = %v, want %v", tc.sql, got, tc.want)
		}
	}
}

func TestSubqueryIgnoresOuterTableLimit(t *testing.T) {
	q, err := query.Parse("SELECT count(*) AS n FROM tasks WHERE file IN (SELECT file FROM frontmatter WHERE fm.status = 'active')")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Execute(q, subqueryDocuments(t), Options{Strict: true, TableLimit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Rows[0]["n"].String(); got != "2" {
		t.Fatalf("count = %s, want 2", got)
	}
}

func TestSubqueryRequiresOneColumnAndNoOuterReferences(t *testing.T) {
	for _, tc := range []struct {
		sql, want string
	}{
		{"SELECT text FROM tasks WHERE file IN (SELECT file, fm.status FROM frontmatter)", "must return exactly one column"},
		{"SELECT text FROM tasks WHERE file IN (SELECT file FROM frontmatter WHERE state = 'todo')", `subquery failed: unknown column "state"`},
	} {
		q, err := query.Parse(tc.sql)
		if err != nil {
			t.Fatal(err)
		}
		_, err = Execute(q, subqueryDocuments(t), Options{Strict: true})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error = %v, want %q", tc.sql, err, tc.want)
		}
	}
}
