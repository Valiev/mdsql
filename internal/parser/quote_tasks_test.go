package parser_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/cache"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/tables"
)

func TestQuotedTasksAndContainers(t *testing.T) {
	lines := []string{
		"# Tasks",
		"> - [ ] Quoted #one [[Link]] 📅 2026-09-05",
		">   - [/] Indented",
		"> > - [x] Nested quote",
		"",
		"> [!note]+ Checklist",
		"> - [-] Cancelled #two",
		"> > - [>] Forward",
		"- [ ] Outside #three",
		`\> - [ ] Escaped quote`,
		`> \> - [ ] Escaped nested quote`,
	}
	raw := strings.Join(lines, "\n")
	doc, err := parser.ParseFile("quotes.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		line, depth            int
		state, text, container string
	}{
		{2, 0, "todo", "Quoted #one Link", "blockquote"},
		{3, 1, "in_progress", "Indented", "blockquote"},
		{4, 0, "done", "Nested quote", "blockquote"},
		{7, 0, "cancelled", "Cancelled #two", "callout"},
		{8, 0, "other", "Forward", "callout"},
		{9, 0, "todo", "Outside #three", ""},
	}
	if len(doc.Tasks) != len(want) {
		t.Fatalf("tasks = %#v", doc.Tasks)
	}
	for i, expected := range want {
		task := doc.Tasks[i]
		if task.Line != expected.line || task.Start != expected.line || task.End != expected.line || task.Raw != lines[expected.line-1] || task.Depth != expected.depth || task.State != expected.state || task.Text != expected.text || task.Container != expected.container || task.Section != "Tasks" {
			t.Errorf("task %d = %#v, want %#v", i, task, expected)
		}
	}
	if doc.Tasks[0].Due == nil || doc.Tasks[0].Due.Format(time.DateOnly) != "2026-09-05" || !reflect.DeepEqual(doc.Tasks[0].Tags, []string{"one"}) || !doc.Tasks[2].Done {
		t.Fatalf("quoted task metadata = %#v", doc.Tasks)
	}
	if len(doc.Tags) != 3 || len(doc.Links) != 1 || doc.Sections[1].Tasks != 6 || doc.Sections[1].OpenTasks != 5 {
		t.Fatalf("quoted source counted incorrectly: tags=%#v links=%#v section=%#v", doc.Tags, doc.Links, doc.Sections[1])
	}
	table, _ := tables.Table("tasks")
	rows := table.Rows(doc)
	if rows[0]["container"].String() != "blockquote" || rows[3]["container"].String() != "callout" || rows[5]["container"].String() != "" {
		t.Fatalf("container column = %#v", rows)
	}
}

func TestQuotedFencesExcludeTasksTagsAndLinks(t *testing.T) {
	for _, tc := range []struct{ name, open, inner, close string }{
		{"triple", "```go", "~~~", "```"},
		{"quadruple", "````markdown", "```", "`````"},
		{"tilde", "~~~text", "```", "~~~"},
		{"trailing text is not close", "```", "```text", "```"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := strings.Join([]string{
				"> [!note]",
				"> " + tc.open,
				"> " + tc.inner,
				"> - [ ] Hidden #hidden [[Hidden]]",
				"> > - [ ] Also hidden #nested [[Nested]]",
				"> " + tc.close,
				"> - [ ] Visible #visible [[Visible]]",
				"- [ ] Outside",
			}, "\n")
			doc, err := parser.ParseFile("quotes.md", raw, int64(len(raw)), time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			if len(doc.Tasks) != 2 || doc.Tasks[0].Line != 7 || doc.Tasks[0].Container != "callout" || doc.Tasks[1].Container != "" {
				t.Fatalf("tasks = %#v", doc.Tasks)
			}
			if len(doc.Tags) != 1 || doc.Tags[0].Tag != "visible" || len(doc.Links) != 1 || doc.Links[0].Target != "Visible" {
				t.Fatalf("code links/tags leaked: %#v %#v", doc.Links, doc.Tags)
			}
		})
	}
}

func TestNestedCalloutAndUnterminatedQuotedFence(t *testing.T) {
	raw := "> > [!warning]\n> > - [ ] Nested callout\n> - [ ] Plain quote\n> > ~~~\n> > - [ ] Hidden\n> - [ ] After nested quote\n> ```\n> - [ ] Hidden to end\n- [ ] Outside"
	doc, err := parser.ParseFile("quotes.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Tasks) != 4 || doc.Tasks[0].Container != "callout" || doc.Tasks[1].Container != "blockquote" || doc.Tasks[2].Line != 6 || doc.Tasks[2].Container != "blockquote" || doc.Tasks[3].Line != 9 {
		t.Fatalf("nested containers = %#v", doc.Tasks)
	}
}

func TestQuotedTasksCacheRoundTrip(t *testing.T) {
	raw := "> [!note]\n> - [ ] Cached #tag [[Link]] 📅 2026-09-05\n> ```\n> - [ ] Hidden #hidden [[Hidden]]\n> ```\n"
	path := filepath.Join(t.TempDir(), "quotes.md")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := parser.ParseFile(path, raw, fi.Size(), fi.ModTime())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	store, err := cache.OpenDefault()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Store(doc); err != nil {
		t.Fatal(err)
	}
	warm, ok := store.Load(path, fi)
	if !ok {
		t.Fatal("cache miss")
	}
	if !reflect.DeepEqual(doc, warm) || len(warm.Tasks) != 1 || warm.Tasks[0].Container != "callout" {
		t.Fatalf("cache changed quoted tasks: cold=%#v warm=%#v", doc.Tasks, warm.Tasks)
	}
}
