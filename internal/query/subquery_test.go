package query

import (
	"strings"
	"testing"
)

func TestParseINSubquery(t *testing.T) {
	q, err := Parse("SELECT file, text FROM tasks WHERE file NOT IN (SELECT file FROM frontmatter WHERE fm.status = 'active')")
	if err != nil {
		t.Fatal(err)
	}
	predicate, ok := q.Where.(*BinaryExpr)
	if !ok || predicate.Op != "NOT IN" {
		t.Fatalf("WHERE = %#v, want NOT IN expression", q.Where)
	}
	subquery, ok := predicate.Right.(*SubqueryExpr)
	if !ok || subquery.Query == nil {
		t.Fatalf("right = %#v, want subquery", predicate.Right)
	}
	if subquery.Query.Table != "frontmatter" || len(subquery.Query.Select) != 1 || subquery.Query.Raw != "SELECT file FROM frontmatter WHERE fm.status = 'active'" {
		t.Fatalf("subquery = %#v", subquery.Query)
	}
}

func TestSubqueryCountAllDoesNotLeakToOuterQuery(t *testing.T) {
	q, err := Parse("SELECT file FROM tasks WHERE file IN (SELECT count(*) FROM frontmatter)")
	if err != nil {
		t.Fatal(err)
	}
	if q.CountAll {
		t.Fatal("inner COUNT(*) marked the outer query as COUNT(*)")
	}
	subquery := q.Where.(*BinaryExpr).Right.(*SubqueryExpr).Query
	if !subquery.CountAll {
		t.Fatal("subquery did not retain COUNT(*) metadata")
	}
}

func TestParseSubqueryDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		sql  string
		want string
	}{
		{"SELECT file FROM tasks WHERE file IN (SELECT file FROM frontmatter", ") after subquery"},
		{"SELECT file FROM tasks WHERE file IN (SELECT file FROM frontmatter IN 'other.md')", "file specifications are not supported inside a subquery"},
	} {
		_, err := Parse(tc.sql)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Parse(%q) error = %v, want %q", tc.sql, err, tc.want)
		}
	}
}
