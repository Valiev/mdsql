package engine_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func TestFrontmatterMatchingSyntheticCorpus(t *testing.T) {
	const matchingDocuments = 89
	const otherDocuments = 7
	docs := make([]*parser.Document, 0, matchingDocuments+otherDocuments)
	wantFiles := make(map[string]bool, matchingDocuments)

	for i := 0; i < matchingDocuments+otherDocuments; i++ {
		name := fmt.Sprintf("synthetic-%03d.md", i)
		category := "[[Projects]]"
		if i < matchingDocuments {
			category = "[[Agent Tasks]]"
			wantFiles[name] = true
		}
		source := fmt.Sprintf("---\ncategories:\n  - %q\nstage: implement\n---\n\n# Synthetic document\n", category)
		doc, err := parser.ParseFile(name, source, int64(len(source)), time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, doc)
	}

	for _, predicate := range []string{
		"raw LIKE '%Agent Tasks%'",
		"contains(fm.categories, '[[Agent Tasks]]')",
	} {
		t.Run(predicate, func(t *testing.T) {
			q, err := query.Parse("SELECT file FROM frontmatter WHERE " + predicate)
			if err != nil {
				t.Fatal(err)
			}
			result, err := engine.Execute(q, docs, engine.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Rows) != matchingDocuments {
				t.Fatalf("got %d rows, want %d", len(result.Rows), matchingDocuments)
			}
			seen := make(map[string]bool, matchingDocuments)
			for _, row := range result.Rows {
				file := row["file"].String()
				if !wantFiles[file] {
					t.Errorf("unexpected result file %q", file)
				}
				if seen[file] {
					t.Errorf("duplicate result file %q", file)
				}
				seen[file] = true
			}
			for file := range wantFiles {
				if !seen[file] {
					t.Errorf("missing result file %q", file)
				}
			}
		})
	}
}
