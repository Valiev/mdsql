package engine_test

import (
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func TestHasTagScope(t *testing.T) {
	source := "---\ntags: [Project/Root]\ntag: legacy\n---\n# Heading #Work/Notes\n\nParagraph #body/tag\ncontinued #multi/child\n\n- [ ] task #todo/kid\n\n```md\n#hidden\n```\n"
	doc, err := parser.ParseFile("synthetic.md", source, int64(len(source)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		table, tag string
		rows       int
	}{
		{"files", "project", 1}, {"frontmatter", "PROJECT/ROOT", 1}, {"files", "legacy", 1},
		{"files", "work", 0}, {"frontmatter", "body", 0}, {"files", "proj", 0},
		{"headings", "work", 1}, {"headings", "Work/Notes", 1}, {"headings", "body", 0},
		{"headings", "project", 0}, {"lines", "work", 1}, {"lines", "body", 1},
		{"blocks", "multi", 1}, {"blocks", "work", 1}, {"blocks", "project", 0},
		{"tasks", "todo", 1}, {"tasks", "project", 0}, {"tasks", "work", 0},
		{"lines", "hidden", 0}, {"blocks", "hidden", 0}, {"lines", "project", 0},
	} {
		sql := "SELECT * FROM " + tc.table + " WHERE has_tag('" + tc.tag + "')"
		q, err := query.Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		result, err := engine.Execute(q, []*parser.Document{doc}, engine.Options{Strict: true})
		if err != nil || len(result.Rows) != tc.rows || len(result.Warnings) != 0 {
			t.Fatalf("%s: result=%+v error=%v", sql, result, err)
		}
		for _, row := range result.Rows {
			if len(row) != len(result.Columns) {
				t.Fatalf("context leaked into result: %v", row)
			}
			if tc.table != "tasks" {
				if _, ok := row["tags"]; ok {
					t.Fatalf("context added a tags column: %v", row)
				}
			}
		}
	}
}

func TestHasTagKeepsSameDisplayPathDocumentsSeparate(t *testing.T) {
	var docs []*parser.Document
	for _, tag := range []string{"one", "two"} {
		source := "---\ntags: [" + tag + "]\n---\n# Heading #" + tag + "\n"
		doc, err := parser.ParseFile("same-display.md", source, int64(len(source)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, doc)
	}
	for _, table := range []string{"files", "frontmatter", "headings", "lines", "blocks"} {
		sql := "SELECT has_tag('one') AS first, has_tag('two') AS second FROM " + table + " WHERE has_tag('one') OR has_tag('two') ORDER BY first DESC"
		q, err := query.Parse(sql)
		if err != nil {
			t.Fatal(err)
		}
		result, err := engine.Execute(q, docs, engine.Options{Strict: true})
		if err != nil || len(result.Rows) != 2 || result.Rows[0]["first"].Data != true || result.Rows[0]["second"].Data != false || result.Rows[1]["first"].Data != false || result.Rows[1]["second"].Data != true {
			t.Fatalf("%s: result=%+v error=%v", sql, result, err)
		}
	}
}
