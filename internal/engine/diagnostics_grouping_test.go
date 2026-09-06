package engine_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func feedbackDocument(t *testing.T, name, frontmatter string) *parser.Document {
	t.Helper()
	text := "---\n" + frontmatter + "\n---\n# Synthetic\n"
	doc, err := parser.ParseFile(name, text, int64(len(text)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func feedbackQuery(t *testing.T, sql string, docs []*parser.Document, strict bool) (*engine.ExecuteResult, error) {
	t.Helper()
	q, err := query.Parse(sql)
	if err != nil {
		t.Fatal(err)
	}
	return engine.Execute(q, docs, engine.Options{Strict: strict})
}

func TestComparisonWarningsAndStrict(t *testing.T) {
	docs := []*parser.Document{
		feedbackDocument(t, "one.md", "categories: ['[[Agent Tasks]]']\nstatus: active"),
		feedbackDocument(t, "two.md", "categories: ['[[Agent Tasks]]']\nstatus: active"),
	}
	for _, predicate := range []string{
		"fm.categories = '[[Agent Tasks]]'",
		"fm.categories != '[[Agent Tasks]]'",
		"fm.categories BETWEEN 1 AND 3",
		"fm.categories LIKE '%Agent%'",
		"contains(fm.categories, fm.categories)",
		"fm.missing = 'anything'",
		"missing = 'anything'",
	} {
		t.Run(predicate, func(t *testing.T) {
			sql := "SELECT file FROM frontmatter WHERE " + predicate
			result, err := feedbackQuery(t, sql, docs, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Rows) != 0 || len(result.Warnings) != 1 {
				t.Fatalf("rows=%d warnings=%v, want no rows and one deduplicated warning", len(result.Rows), result.Warnings)
			}
			if _, err := feedbackQuery(t, sql, docs, true); err == nil {
				t.Fatal("strict query must fail")
			}
		})
	}
}

func TestUnknownWhereColumnWithoutRows(t *testing.T) {
	sql := "SELECT file FROM frontmatter WHERE unknown_field = 'x'"
	result, err := feedbackQuery(t, sql, nil, false)
	if err != nil || len(result.Warnings) != 1 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if _, err := feedbackQuery(t, sql, nil, true); err == nil {
		t.Fatal("strict empty query must diagnose its unknown WHERE column")
	}
}

func TestOptionalFieldsAndExplicitNull(t *testing.T) {
	docs := []*parser.Document{feedbackDocument(t, "one.md", "owner: demo\nexplicit_null: null\nscalar: text")}
	for _, predicate := range []string{
		"fm.missing IS NULL",
		"coalesce(fm.missing, 'fallback') = 'fallback'",
		"fm.missing IS NOT NULL AND fm.missing = 'x'",
		"fm.missing IS NULL OR fm.missing = 'x'",
		"fm.explicit_null = 'x'",
	} {
		t.Run(predicate, func(t *testing.T) {
			result, err := feedbackQuery(t, "SELECT file FROM frontmatter WHERE "+predicate, docs, true)
			if err != nil || len(result.Warnings) != 0 {
				t.Fatalf("result=%#v error=%v", result, err)
			}
		})
	}
	result, err := feedbackQuery(t, "SELECT fm.missing FROM frontmatter", docs, true)
	if err != nil || result.Rows[0]["fm.missing"].Kind != query.ValueNull {
		t.Fatalf("missing projection must be NULL, result=%#v err=%v", result, err)
	}
	if _, err := feedbackQuery(t, "SELECT file FROM frontmatter WHERE fm.scalar.nested = 'x'", docs, true); err == nil {
		t.Fatal("nested lookup through a scalar must diagnose a missing field, not panic")
	}
}

func TestFrontmatterGroupingAndAliases(t *testing.T) {
	docs := []*parser.Document{
		feedbackDocument(t, "a.md", "stage: alpha"),
		feedbackDocument(t, "b.md", "stage: beta"),
		feedbackDocument(t, "c.md", "stage: alpha"),
		feedbackDocument(t, "d.md", "owner: demo"),
		feedbackDocument(t, "e.md", "stage: null"),
	}
	tests := []struct{ sql, group, count string }{
		{"SELECT fm.stage, COUNT(*) FROM frontmatter GROUP BY fm.stage ORDER BY fm.stage", "fm.stage", "count"},
		{"SELECT fm.stage AS s, COUNT(*) FROM frontmatter GROUP BY s ORDER BY s", "s", "count"},
		{"SELECT fm.stage AS s, COUNT(*) AS n FROM frontmatter GROUP BY fm.stage ORDER BY s", "s", "n"},
	}
	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			result, err := feedbackQuery(t, tt.sql, docs, true)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Rows) != 3 {
				t.Fatalf("got %d groups, want 3", len(result.Rows))
			}
			wantNames := []string{"alpha", "beta", ""}
			wantCounts := []int64{2, 1, 2}
			for i, row := range result.Rows {
				if len(row) != 2 || row[tt.group].String() != wantNames[i] || row[tt.count].Data.(int64) != wantCounts[i] {
					t.Fatalf("unexpected group %d: %#v", i, row)
				}
			}
			if result.Rows[2][tt.group].Kind != query.ValueNull {
				t.Fatal("NULL group was converted to a string")
			}
		})
	}
	result, err := feedbackQuery(t, "SELECT fm.stage AS s, COUNT(*) AS n FROM frontmatter GROUP BY s ORDER BY n DESC, s LIMIT 1 OFFSET 1", docs, true)
	if err != nil || len(result.Rows) != 1 || result.Rows[0]["s"].Kind != query.ValueNull {
		t.Fatalf("grouped order/limit/offset failed: result=%#v error=%v", result, err)
	}
	result, err = feedbackQuery(t, "SELECT COUNT(*) AS total FROM frontmatter", docs, true)
	if err != nil || len(result.Rows[0]) != 1 || result.Rows[0]["total"].Data.(int64) != 5 {
		t.Fatalf("COUNT alias failed: result=%#v error=%v", result, err)
	}
}

func TestGroupKeysPreserveTypesAndNestedPaths(t *testing.T) {
	docs := []*parser.Document{
		feedbackDocument(t, "a.md", "meta:\n  stage: 1"),
		feedbackDocument(t, "b.md", "meta:\n  stage: '1'"),
	}
	result, err := feedbackQuery(t, "SELECT fm.meta.stage AS s, COUNT(*) FROM frontmatter GROUP BY s ORDER BY s", docs, true)
	if err != nil || len(result.Rows) != 2 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if result.Rows[0]["s"].Kind == result.Rows[1]["s"].Kind {
		t.Fatal("integer and string groups lost their types")
	}
	result, err = feedbackQuery(t, "SELECT fm.meta.stage AS s FROM frontmatter ORDER BY s DESC", docs, true)
	if err != nil || len(result.Rows) != 2 || result.Rows[0]["s"].Kind != query.ValueInt {
		t.Fatalf("ORDER BY alias failed: result=%#v error=%v", result, err)
	}
}

func TestFrontmatterColumnHints(t *testing.T) {
	docs := []*parser.Document{feedbackDocument(t, "one.md", "status: active")}
	for _, sql := range []string{
		"SELECT status FROM frontmatter",
		"SELECT file FROM frontmatter WHERE status = 'active'",
		"SELECT file FROM frontmatter ORDER BY status",
		"SELECT COUNT(*) FROM frontmatter GROUP BY status",
	} {
		_, err := feedbackQuery(t, sql, docs, true)
		if err == nil || !strings.Contains(err.Error(), "fm.status") {
			t.Fatalf("%s: expected fm.status hint, got %v", sql, err)
		}
	}
	if _, err := feedbackQuery(t, "SELECT missing AS missing FROM frontmatter", docs, false); err == nil {
		t.Fatal("a SELECT alias must not hide its unknown source column")
	}
}

func TestOrderByPropagatesStrictDiagnostics(t *testing.T) {
	docs := []*parser.Document{feedbackDocument(t, "one.md", "categories: [demo]")}
	_, err := feedbackQuery(t, "SELECT file FROM frontmatter ORDER BY fm.categories = 'demo'", docs, true)
	if err == nil || !strings.Contains(err.Error(), "ORDER BY") {
		t.Fatalf("expected strict ordering error, got %v", err)
	}
}
