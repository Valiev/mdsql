package engine

import (
	"reflect"
	"testing"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func TestStreamingExecutorMatchesBatchExecution(t *testing.T) {
	docs := []*model.Document{
		{
			Path:        "b.md",
			FrontMatter: model.FrontMatter{"stage": "verify", "owner": "alice"},
			Tasks: []model.Task{
				{Block: model.Block{File: "b.md", Start: 8, End: 8, Raw: "- [ ] Second"}, Text: "Second", Status: " ", State: "todo"},
				{Block: model.Block{File: "b.md", Start: 4, End: 4, Raw: "- [x] First"}, Text: "First", Status: "x", State: "done", Done: true},
			},
		},
		{
			Path:        "a.md",
			FrontMatter: model.FrontMatter{"stage": "verify", "owner": "bob"},
			Tasks:       []model.Task{{Block: model.Block{File: "a.md", Start: 2, End: 2, Raw: "- [ ] Second"}, Text: "Second", Status: " ", State: "todo"}},
		},
	}
	queries := []string{
		"SELECT file, text FROM tasks WHERE state = 'todo'",
		"SELECT text FROM tasks ORDER BY text DESC, file LIMIT 2 OFFSET 1",
		"SELECT DISTINCT text FROM tasks ORDER BY file",
		"SELECT fm.stage AS stage, count(*) AS n FROM frontmatter GROUP BY stage HAVING n > 1",
		"SELECT file, text FROM tasks ORDER BY line DESC LIMIT 1 PER file",
	}
	for _, sql := range queries {
		t.Run(sql, func(t *testing.T) {
			q, err := query.Parse(sql)
			if err != nil {
				t.Fatal(err)
			}
			batch, err := Execute(q, docs, Options{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			stream, err := NewExecutor(q, Options{Strict: true})
			if err != nil {
				t.Fatal(err)
			}
			for _, doc := range docs {
				if err := stream.ConsumeDocument(doc); err != nil {
					t.Fatal(err)
				}
			}
			got, err := stream.Finish()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, batch) {
				t.Fatalf("stream = %#v, batch = %#v", got, batch)
			}
		})
	}
}

func TestStreamingExecutorCopiesRetainedValues(t *testing.T) {
	q, err := query.Parse("SELECT fm FROM frontmatter")
	if err != nil {
		t.Fatal(err)
	}
	doc := &model.Document{Path: "note.md", FrontMatter: model.FrontMatter{"owner": "alice", "nested": []interface{}{"one"}}}
	executor, err := NewExecutor(q, Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.ConsumeDocument(doc); err != nil {
		t.Fatal(err)
	}
	doc.FrontMatter["owner"] = "bob"
	doc.FrontMatter["nested"].([]interface{})[0] = "two"
	result, err := executor.Finish()
	if err != nil {
		t.Fatal(err)
	}
	fm := result.Rows[0]["fm"].Data.(map[string]query.Value)
	if fm["owner"].String() != "alice" || fm["nested"].Data.([]query.Value)[0].String() != "one" {
		t.Fatalf("retained values changed with document: %#v", fm)
	}
}

func TestStreamingExecutorDefersOptionalFieldDiagnostic(t *testing.T) {
	q, err := query.Parse("SELECT file FROM files WHERE fm.stage = 'verify'")
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewExecutor(q, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range []*model.Document{{Path: "empty.md"}, {Path: "present.md", FrontMatter: model.FrontMatter{"stage": "verify"}}} {
		if err := executor.ConsumeDocument(doc); err != nil {
			t.Fatal(err)
		}
	}
	result, err := executor.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 || len(result.Rows) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}
