package edit

import (
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/query"
)

func TestReviewExactStructuralLine(t *testing.T) {
	for _, raw := range []string{"# Before\n# After", "# Before\r\n# After", "# Before", "# Before\n"} {
		for _, sql := range []string{
			"INSERT INTO tasks (text,line) VALUES ('new',2)",
			"INSERT INTO sections (level,title,line) VALUES (2,'New',2)",
			"INSERT INTO code (code,line) VALUES ('payload',2)",
		} {
			got, _, _ := corePlan(t, raw, sql)
			doc := coreDoc(t, got)
			found := false
			switch {
			case strings.Contains(sql, "INTO tasks"):
				for _, task := range doc.Tasks {
					if task.Text == "new" {
						found = true
						if task.Line != 2 {
							t.Fatalf("task.Line=%d: %q", task.Line, got)
						}
					}
				}
			case strings.Contains(sql, "INTO sections"):
				for _, heading := range doc.Headings {
					if heading.Title == "New" {
						found = true
						if heading.Start != 2 {
							t.Fatalf("heading.Start=%d: %q", heading.Start, got)
						}
					}
				}
			default:
				for _, block := range doc.Blocks {
					if block.Kind == model.KindCode {
						found = true
						if block.Start != 2 {
							t.Fatalf("code.Start=%d: %q", block.Start, got)
						}
					}
				}
			}
			if !found {
				t.Fatalf("missing inserted target: %q", got)
			}
		}
	}
}

func TestReviewLinesNormalizeOnlyInsertedEOL(t *testing.T) {
	raw := "\ufefffirst\r\nlast\nuntouched"
	stmt := &query.WriteStatement{Operation: "INSERT", Table: "lines", Values: map[string]query.Value{"text": query.NewString("one\ntwo\rthree\r\n"), "line": query.NewInt(2)}}
	got, _, _, err := planText([]byte(raw), stmt, coreDoc(t, raw))
	want := "\ufefffirst\r\none\r\ntwo\r\nthree\r\nlast\nuntouched"
	if err != nil || string(got) != want {
		t.Fatalf("got %q, %v; want %q", got, err, want)
	}
}

func TestReviewRejectTaskMarkers(t *testing.T) {
	for _, text := range []string{"- [ ] foo", "* foo", "+ foo", "1. foo", "2) foo", "[x] foo", "  [ ] foo", "\t- foo"} {
		stmt := &query.WriteStatement{Operation: "INSERT", Table: "tasks", Values: map[string]query.Value{"text": query.NewString(text)}}
		if _, _, _, err := planText(nil, stmt, coreDoc(t, "")); err == nil {
			t.Fatalf("accepted marker in task text %q", text)
		}
	}
	corePlan(t, "", "INSERT INTO tasks (text) VALUES ('content with - punctuation and [link]')")
}

func TestReviewTaskMarkerInference(t *testing.T) {
	for _, tc := range []struct{ raw, sql, marker string }{
		{"* [ ] old", "INSERT INTO tasks (text,line) VALUES ('new',2)", "* "},
		{"+ [ ] old", "INSERT INTO tasks (text,line) VALUES ('new',1)", "+ "},
		{"1. [ ] old", "INSERT INTO tasks (text,line) VALUES ('new',2)", "1. "},
		{"*   [ ] old", "INSERT INTO tasks (text,line) VALUES ('new',2)", "*   "},
		{"* [ ] parent\n  + [ ] child\n    - [ ] deeper", "INSERT INTO tasks (text,parent_line) VALUES ('new',1)", "  + "},
		{"* [ ] parent", "INSERT INTO tasks (text,parent_line) VALUES ('new',1)", "  * "},
		{"+ [ ] unrelated\n# New", "INSERT INTO tasks (text) VALUES ('new')", "- "},
		{"```\n* [ ] not a task\n```", "INSERT INTO tasks (text) VALUES ('new')", "- "},
	} {
		got, _, _ := corePlan(t, tc.raw, tc.sql)
		found := false
		for _, task := range coreDoc(t, got).Tasks {
			if task.Text == "new" {
				found = true
				if task.Raw != tc.marker+"[ ] new" {
					t.Fatalf("marker got %q want %q", task.Raw, tc.marker+"[ ] new")
				}
			}
		}
		if !found {
			t.Fatalf("missing task: %q", got)
		}
	}
}

func TestReviewCoreStatusMatchesSQL(t *testing.T) {
	for _, status := range []string{" ", "x", "X", "/", "-", "?", "✓", "", "xx"} {
		values := map[string]query.Value{"status": query.NewString(status)}
		_, coreErr := taskStatus(values)
		_, sqlErr := query.ParseWrite("UPDATE tasks SET status = '" + status + "' WHERE line = 1")
		if (coreErr == nil) != (sqlErr == nil) {
			t.Fatalf("status %q differs: core=%v SQL=%v", status, coreErr, sqlErr)
		}
	}
}

func TestReviewSourceNameOverridesOnlyQueryIdentity(t *testing.T) {
	path := coreFile(t, "- [ ] task")
	stmt := coreStmt(t, "UPDATE tasks SET state = 'done' WHERE file = 'note.md'")
	result, err := Execute(path, stmt, Options{DryRun: true, SourceName: "note.md"})
	if err != nil || result.Matched != 1 || result.Changed != 1 || result.Applied || result.File == "note.md" {
		t.Fatalf("source identity %#v %v", result, err)
	}
	if coreContents(t, path) != "- [ ] task" {
		t.Fatal("preview changed physical source")
	}
	coreNoArtifacts(t, path)
	result, err = Execute(path, stmt, Options{DryRun: true})
	if err != nil || result.Matched != 0 {
		t.Fatalf("unoverridden identity %#v %v", result, err)
	}
}
