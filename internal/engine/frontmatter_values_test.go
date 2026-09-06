package engine_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func TestFrontmatterValuesQueries(t *testing.T) {
	sources := []struct{ name, text string }{
		{"a.md", "---\ncategories: ['[[Agent Tasks]]', '[[Projects]]', '[[Agent Tasks]]']\nmeta: {stage: plan}\nempty: []\nexplicit_null: null\nliteral.key: yes\n---\n# Synthetic\n"},
		{"b.md", "---\ncategories: '[[Agent Tasks]]'\nmeta: {stage: implement}\n---\n# Synthetic\n"},
		{"c.md", "---\ncategories: ['[[Projects]]']\n---\n# Synthetic\n"},
		{"plain.md", "# No frontmatter\n"},
		{"empty.md", "---\n---\n# Empty frontmatter\n"},
	}
	var docs []*parser.Document
	for _, source := range sources {
		doc, err := parser.ParseFile(source.name, source.text, int64(len(source.text)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, doc)
	}
	tests := []struct {
		name string
		sql  string
		want []query.Row
	}{
		{
			name: "membership",
			sql:  "SELECT DISTINCT file FROM frontmatter_values WHERE key = 'categories' AND value = '[[Agent Tasks]]' ORDER BY file",
			want: []query.Row{{"file": query.NewString("a.md")}, {"file": query.NewString("b.md")}},
		},
		{
			name: "group duplicates",
			sql:  "SELECT value, COUNT(*) AS n FROM frontmatter_values WHERE key = 'categories' GROUP BY value ORDER BY value",
			want: []query.Row{
				{"value": query.NewString("[[Agent Tasks]]"), "n": query.NewInt(3)},
				{"value": query.NewString("[[Projects]]"), "n": query.NewInt(2)},
			},
		},
		{
			name: "object path",
			sql:  "SELECT file, value.stage AS stage FROM frontmatter_values WHERE key = 'meta' ORDER BY file",
			want: []query.Row{
				{"file": query.NewString("a.md"), "stage": query.NewString("plan")},
				{"file": query.NewString("b.md"), "stage": query.NewString("implement")},
			},
		},
		{
			name: "explicit null",
			sql:  "SELECT file, key, value FROM frontmatter_values WHERE value IS NULL",
			want: []query.Row{{"file": query.NewString("a.md"), "key": query.NewString("explicit_null"), "value": query.NewNull()}},
		},
		{
			name: "literal dotted key",
			sql:  "SELECT value FROM frontmatter_values WHERE key = 'literal.key'",
			want: []query.Row{{"value": query.NewString("yes")}},
		},
		{
			name: "empty list and absent frontmatter",
			sql:  "SELECT file, key, value FROM frontmatter_values WHERE key = 'empty' OR file = 'plain.md' OR file = 'empty.md'",
			want: []query.Row{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := query.Parse(tt.sql)
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.Execute(q, docs, engine.Options{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.Rows, tt.want) {
				t.Fatalf("rows = %#v, want %#v", result.Rows, tt.want)
			}
		})
	}
}
