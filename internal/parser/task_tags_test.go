package parser_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/engine"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func TestTaskTagsPreserveSourceOccurrences(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		tags       []string
	}{
		{"one", "Task #work", []string{"work"}},
		{"multiple", "Task #work #home/desk #задача", []string{"work", "home/desk", "задача"}},
		{"repeated", "Task #work #work", []string{"work", "work"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := "- [ ] " + tc.text
			doc, err := parser.ParseFile("tasks.md", raw, int64(len(raw)), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Tasks) != 1 || !reflect.DeepEqual(doc.Tasks[0].Tags, tc.tags) {
				t.Fatalf("tasks = %#v, want tags %#v", doc.Tasks, tc.tags)
			}
			if len(doc.Tags) != len(tc.tags) {
				t.Fatalf("tag table occurrences = %#v, want %#v", doc.Tags, tc.tags)
			}
			for i, tag := range doc.Tags {
				if tag.Tag != tc.tags[i] || tag.Line != 1 || tag.Source != "body" {
					t.Errorf("tag %d = %#v", i, tag)
				}
			}
			for _, predicate := range []string{"has_tag('work')", "contains(tags, 'work')"} {
				q, err := query.Parse("SELECT tags FROM tasks WHERE " + predicate)
				if err != nil {
					t.Fatal(err)
				}
				result, err := engine.Execute(q, []*parser.Document{doc}, engine.Options{Strict: true})
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Rows) != 1 || len(result.Rows[0]["tags"].Data.([]query.Value)) != len(tc.tags) {
					t.Fatalf("%s result = %#v", predicate, result)
				}
			}
		})
	}
}

func TestTaskTagsWithStatesAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		marker, state string
		done          bool
	}{
		{" ", "todo", false},
		{"x", "done", true},
		{"X", "done", true},
		{"/", "in_progress", false},
		{"-", "cancelled", false},
		{"?", "other", false},
	} {
		t.Run(tc.state+tc.marker, func(t *testing.T) {
			raw := "- [" + tc.marker + "] Task #work 📅 2026-09-04 ⏫ [points:: 3]"
			doc, err := parser.ParseFile("tasks.md", raw, int64(len(raw)), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Tasks) != 1 {
				t.Fatalf("tasks = %#v", doc.Tasks)
			}
			task := doc.Tasks[0]
			if !reflect.DeepEqual(task.Tags, []string{"work"}) || task.State != tc.state || task.Done != tc.done || task.Status != tc.marker {
				t.Fatalf("task state/tags = %#v", task)
			}
			if task.Due == nil || task.Due.Format(time.DateOnly) != "2026-09-04" || task.Priority != 4 || task.Fields["points"] != 3 || task.Text != "Task #work" {
				t.Fatalf("task metadata = %#v", task)
			}
		})
	}
}
