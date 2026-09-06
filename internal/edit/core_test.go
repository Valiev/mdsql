package edit

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Valiev/mdsql/internal/model"
	"github.com/Valiev/mdsql/internal/parser"
	"github.com/Valiev/mdsql/internal/query"
)

func coreDoc(t *testing.T, raw string) *model.Document {
	t.Helper()
	doc, err := parser.ParseFile("note.md", raw, int64(len(raw)), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
func coreStmt(t *testing.T, sql string) *query.WriteStatement {
	t.Helper()
	stmt, err := query.ParseWrite(sql)
	if err != nil {
		t.Fatal(err)
	}
	return stmt
}
func corePlan(t *testing.T, raw, sql string) (string, int, int) {
	t.Helper()
	stmt := coreStmt(t, sql)
	if err := validateStatement(stmt); err != nil {
		t.Fatal(err)
	}
	after, matched, changed, err := planText([]byte(raw), stmt, coreDoc(t, raw))
	if err != nil {
		t.Fatal(err)
	}
	return string(after), matched, changed
}

func TestCoreLogicalLines(t *testing.T) {
	for _, tc := range []struct {
		name, raw, text, want string
		line                  int
	}{
		{"empty", "", "new", "new", 1},
		{"bom only", "\ufeff", "new", "\ufeffnew", 1},
		{"append no LF", "old", "new", "old\nnew", 2},
		{"append LF", "old\n", "new", "old\nnew\n", 2},
		{"final input LF no phantom", "old", "one\ntwo\n", "old\none\ntwo", 2},
		{"middle", "first\nlast", "new", "first\nnew\nlast", 2},
		{"CRLF", "first\r\nlast", "new", "first\r\nlast\r\nnew", 3},
		{"BOM mixed", "\ufefffirst\r\nlast\n", "new", "\ufefffirst\r\nnew\r\nlast\n", 2},
		{"normalized inserted EOL", "old\r\n", "a\nb\r\nc", "a\r\nb\r\nc\r\nold\r\n", 1},
		{"inside fence", "```\nx\n```", "raw", "```\nraw\nx\n```", 2},
		{"bare CR", "a\rb", "c", "a\rc\rb", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stmt := &query.WriteStatement{Operation: "INSERT", Table: "lines", Values: map[string]query.Value{"text": query.NewString(tc.text), "line": query.NewInt(int64(tc.line))}}
			got, m, c, err := planText([]byte(tc.raw), stmt, coreDoc(t, tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want || m != 1 || c != 1 {
				t.Fatalf("got %q (%d,%d), want %q", got, m, c, tc.want)
			}
		})
	}
}

func TestCoreLineInsertProtectsFrontmatter(t *testing.T) {
	for _, tc := range []struct {
		name, raw, sql, wantError string
	}{
		{"first line", "---\nstage: plan\n---\nbody", "INSERT INTO lines (line,text) VALUES (1,'x')", "inside frontmatter"},
		{"inside YAML", "---\nstage: plan\n---\nbody", "INSERT INTO lines (line,text) VALUES (2,'x')", "inside frontmatter"},
		{"CRLF", "---\r\nstage: plan\r\n---\r\nbody", "INSERT INTO lines (line,text) VALUES (2,'x')", "inside frontmatter"},
		{"BOM", "\ufeff---\nstage: plan\n---\nbody", "INSERT INTO lines (line,text) VALUES (1,'x')", "inside frontmatter"},
		{"new invalid YAML", "body", "INSERT INTO lines (line,text) VALUES (1,'---')", "invalid frontmatter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stmt := coreStmt(t, tc.sql)
			_, _, _, err := planText([]byte(tc.raw), stmt, coreDoc(t, tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("got %v, want error containing %q", err, tc.wantError)
			}
		})
	}
	got, matched, changed := corePlan(t, "body", "INSERT INTO lines (line,text) VALUES (1,'x')")
	if got != "x\nbody" || matched != 1 || changed != 1 {
		t.Fatalf("file without frontmatter: got %q (%d,%d)", got, matched, changed)
	}
}

func TestCoreTaskUpdateOriginalOffsets(t *testing.T) {
	raw := "\ufeff- [✓] one\r\n> 1. [ ] two\n> > + [X] three\r\n- [-] four"
	after, m, c := corePlan(t, raw, "UPDATE tasks SET status = 'X' WHERE state != 'cancelled'")
	want := "\ufeff- [X] one\r\n> 1. [X] two\n> > + [X] three\r\n- [-] four"
	if after != want || m != 3 || c != 2 {
		t.Fatalf("got %q (%d,%d)", after, m, c)
	}
	after, m, c = corePlan(t, after, "UPDATE tasks SET status = 'X' WHERE state = 'done'")
	if after != want || m != 3 || c != 0 {
		t.Fatalf("no-op got %q (%d,%d)", after, m, c)
	}
}

func TestCoreTaskStatesAndStrictSelect(t *testing.T) {
	for state, status := range map[string]string{"todo": " ", "done": "x", "in_progress": "/", "cancelled": "-"} {
		got, m, c := corePlan(t, "- [?] one\n- [?] two", "UPDATE tasks SET state = '"+state+"' WHERE state = 'other'")
		if got != "- ["+status+"] one\n- ["+status+"] two" || m != 2 || c != 2 {
			t.Fatalf("state %s: %q %d %d", state, got, m, c)
		}
	}
	for _, sql := range []string{"UPDATE tasks SET state = 'done' WHERE typo = 1", "UPDATE tasks SET state = 'done' WHERE line = 'bad'"} {
		stmt := coreStmt(t, sql)
		if _, _, _, err := planText([]byte("- [ ] task"), stmt, coreDoc(t, "- [ ] task")); err == nil {
			t.Fatalf("expected strict error: %s", sql)
		}
	}
	raw := strings.Repeat("- [ ] task\n", 1200)
	_, m, c := corePlan(t, raw, "UPDATE tasks SET state = 'done' WHERE state = 'todo'")
	if m != 1200 || c != 1200 {
		t.Fatalf("unexpected row cap %d %d", m, c)
	}
	today := time.Now().Format(time.DateOnly)
	raw = "- [ ] task [due:: " + today + "]"
	_, m, _ = corePlan(t, raw, "UPDATE tasks SET state = 'done' WHERE due <= today")
	if m != 1 {
		t.Fatal("default date context was not populated")
	}
}

func TestCoreStructuralInsertions(t *testing.T) {
	for _, tc := range []struct{ name, raw, sql, want string }{
		{"task empty", "", "INSERT INTO tasks (text) VALUES ('new')", "- [ ] new"},
		{"task EOF", "old", "INSERT INTO tasks (text,state) VALUES ('new','done')", "old\n\n- [x] new"},
		{"task CRLF BOM", "\ufeff# Top\r\n", "INSERT INTO tasks (text,status) VALUES ('new','X')", "\ufeff# Top\r\n\r\n- [X] new\r\n"},
		{"section", "old", "INSERT INTO sections (level,title) VALUES (2,'New')", "old\n\n## New"},
		{"section own", "# A\nbody\n\n## Child\nchild\n# B\n", "INSERT INTO tasks (text,section) VALUES ('new','A')", "# A\nbody\n\n- [ ] new\n\n## Child\nchild\n# B\n"},
		{"section path", "# A\n## Child\n# B\n## Child\n", "INSERT INTO lines (text,section_path) VALUES ('new','A > Child')", "# A\n## Child\nnew\n# B\n## Child\n"},
		{"heading disambiguation", "# A\n# A\n", "INSERT INTO tasks (text,heading_line) VALUES ('new',1)", "# A\n\n- [ ] new\n\n# A\n"},
		{"reparent downstream", "# A\n\n## Child\n", "INSERT INTO sections (level,title,section) VALUES (1,'B','A')", "# A\n\n# B\n\n## Child\n"},
		{"after nested subtree", "- [ ] parent\n  - [ ] child\n    - [ ] grandchild\n  - plain\n    continuation\n\n- [ ] next", "INSERT INTO tasks (text,parent_line) VALUES ('new',1)", "- [ ] parent\n  - [ ] child\n    - [ ] grandchild\n  - plain\n    continuation\n  - [ ] new\n\n- [ ] next"},
		{"four space infer", "- [ ] parent\n    - [ ] child\n        child continuation\n- [ ] next", "INSERT INTO tasks (text,parent_line) VALUES ('new',1)", "- [ ] parent\n    - [ ] child\n        child continuation\n    - [ ] new\n- [ ] next"},
		{"new child no LF", "- [ ] parent", "INSERT INTO tasks (text,parent_line) VALUES ('new',1)", "- [ ] parent\n  - [ ] new"},
		{"setext own", "Title\n=====\n\n# Next", "INSERT INTO tasks (text,heading_line) VALUES ('new',1)", "Title\n=====\n\n- [ ] new\n\n# Next"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, m, c := corePlan(t, tc.raw, tc.sql)
			if got != tc.want || m != 1 || c != 1 {
				t.Fatalf("got %q (%d,%d), want %q", got, m, c, tc.want)
			}
		})
	}
}

func TestCoreSectionSelectorAdviceMatchesFailure(t *testing.T) {
	raw := "# Present\n# Duplicate\n# Duplicate\n"
	for _, tc := range []struct {
		sql      string
		want     string
		unwanted string
	}{
		{"INSERT INTO tasks (text,section) VALUES ('new','Missing')", "check the section title or path", "heading_line to disambiguate"},
		{"INSERT INTO tasks (text,section) VALUES ('new','Duplicate')", "heading_line to disambiguate", "matched no section"},
	} {
		stmt := coreStmt(t, tc.sql)
		_, _, _, err := planText([]byte(raw), stmt, coreDoc(t, raw))
		if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), tc.unwanted) {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestCoreFencesRoundTrip(t *testing.T) {
	for _, code := range []string{"", "hello", "hello\n", "```\n# hidden\n````\n- [ ] hidden", "  `````` \t\n~~~\ncode", "\n"} {
		stmt := &query.WriteStatement{Operation: "INSERT", Table: "code", Values: map[string]query.Value{"code": query.NewString(code), "lang": query.NewString("Go"), "line": query.NewInt(2)}}
		raw := "# Before\r\n# After"
		got, _, _, err := planText([]byte(raw), stmt, coreDoc(t, raw))
		if err != nil {
			t.Fatalf("code %q: %v", code, err)
		}
		doc := coreDoc(t, string(got))
		if len(doc.Headings) != 2 || len(doc.Tasks) != 0 {
			t.Fatalf("payload leaked: %#v", doc)
		}
		found := false
		for _, b := range doc.Blocks {
			if b.Kind == model.KindCode {
				found = true
				if b.Text != code || b.Lang != "go" {
					t.Fatalf("code roundtrip got %q want %q", b.Text, code)
				}
			}
		}
		if !found {
			t.Fatal("missing code")
		}
	}
}

func TestCoreRejectUnsafePositions(t *testing.T) {
	for _, tc := range []struct{ raw, sql string }{
		{"# A\n# A", "INSERT INTO tasks (text,section) VALUES ('new','A')"},
		{"# A\n## B\n# A\n## B", "INSERT INTO tasks (text,section_path) VALUES ('new','A > B')"},
		{"x\n", "INSERT INTO lines (text,line) VALUES ('new',3)"},
		{"", "INSERT INTO lines (text) VALUES ('')"},
		{"```go\nbody", "INSERT INTO tasks (text) VALUES ('new')"},
		{"```", "INSERT INTO tasks (text) VALUES ('new')"},
		{"```\nbody\n```", "INSERT INTO tasks (text,line) VALUES ('new',2)"},
		{"---\na: 1\n---", "INSERT INTO tasks (text,line) VALUES ('new',2)"},
		{"---\na: [\n---", "INSERT INTO tasks (text) VALUES ('new')"},
		{"---\na: 1", "INSERT INTO tasks (text) VALUES ('new')"},
		{"| A |\n| - |\n| x |", "INSERT INTO tasks (text,line) VALUES ('new',2)"},
		{"> first\n> second", "INSERT INTO tasks (text,line) VALUES ('new',2)"},
		{"first\nsecond", "INSERT INTO tasks (text,line) VALUES ('new',2)"},
		{"<!-- unfinished", "INSERT INTO tasks (text) VALUES ('new')"},
		{"$$\nx", "INSERT INTO tasks (text) VALUES ('new')"},
		{"> - [ ] parent", "INSERT INTO tasks (text,parent_line) VALUES ('new',1)"},
		{"- [ ] parent\n\t- [ ] child", "INSERT INTO tasks (text,parent_line) VALUES ('new',1)"},
		{"# A", "INSERT INTO sections (level,title,body) VALUES (2,'B','```unterminated')"},
	} {
		t.Run(tc.raw+tc.sql, func(t *testing.T) {
			stmt := coreStmt(t, tc.sql)
			if _, _, _, err := planText([]byte(tc.raw), stmt, coreDoc(t, tc.raw)); err == nil {
				t.Fatal("expected unsafe insertion error")
			}
		})
	}
}

func coreFile(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte(raw), 0640); err != nil {
		t.Fatal(err)
	}
	return path
}
func coreContents(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func coreNoArtifacts(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("unexpected write artifacts: %v", entries)
	}
}

func TestCoreExecuteDryRunGuardsNoop(t *testing.T) {
	path := coreFile(t, "- [ ] one\r\n- [X] two")
	before, _ := os.Stat(path)
	original := coreContents(t, path)
	stmt := coreStmt(t, "UPDATE tasks SET state = 'done' WHERE state = 'todo'")
	one, two, negative := 1, 2, -1
	result, err := Execute(path, stmt, Options{DryRun: true, ExpectRows: &one, ExpectSHA256: digest([]byte(original))})
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.Matched != 1 || result.Changed != 1 || result.Diff == "" || result.BaseSHA256 == result.ResultSHA256 {
		t.Fatalf("bad preview %#v", result)
	}
	for _, opts := range []Options{{ExpectRows: &two}, {ExpectRows: &negative}, {ExpectSHA256: strings.Repeat("0", 64)}, {ExpectSHA256: "bad"}} {
		if _, err := Execute(path, stmt, opts); err == nil {
			t.Fatal("guard should reject")
		}
	}
	result, err = Execute(path, coreStmt(t, "UPDATE tasks SET status = 'X' WHERE state = 'done'"), Options{})
	if err != nil || result.Applied || result.Matched != 1 || result.Changed != 0 || result.Diff != "" {
		t.Fatalf("no-op %#v %v", result, err)
	}
	zero := 0
	result, err = Execute(path, coreStmt(t, "UPDATE tasks SET state = 'done' WHERE line = 999"), Options{ExpectRows: &zero})
	if err != nil || result.Applied || result.Matched != 0 || result.Changed != 0 {
		t.Fatalf("zero match %#v %v", result, err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) || coreContents(t, path) != original {
		t.Fatal("preview/guard/no-op wrote source")
	}
	coreNoArtifacts(t, path)
}

func TestCoreExecuteApplyAndIdentity(t *testing.T) {
	path := coreFile(t, "- [ ] task")
	stmt := coreStmt(t, "UPDATE tasks SET state = 'done' WHERE state = 'todo'")
	result, err := Execute(path, stmt, Options{})
	if !atomicWritesSupported {
		if err == nil || result == nil || result.Applied {
			t.Fatalf("expected unsupported write %#v %v", result, err)
		}
		coreNoArtifacts(t, path)
		return
	}
	if err != nil || !result.Applied || coreContents(t, path) != "- [x] task" {
		t.Fatalf("apply %#v %v", result, err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0640 {
		t.Fatalf("mode %v", info.Mode())
	}
	coreNoArtifacts(t, path)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		t.Fatal(err)
	}
	where := &query.BinaryExpr{Op: "=", Left: &query.ColumnExpr{Name: "file"}, Right: &query.LiteralExpr{Value: query.NewString(filepath.Clean(rel))}}
	stmt.Where = where
	result, err = Execute(rel, stmt, Options{DryRun: true})
	if err != nil || result.Matched != 1 {
		t.Fatalf("relative identity %#v %v", result, err)
	}
}

func TestCoreWriterConflicts(t *testing.T) {
	if !atomicWritesSupported {
		t.Skip("atomic writes unavailable")
	}
	for _, kind := range []string{"content same size and time", "replacement inode", "mode", "symlink", "hardlink", "growth"} {
		t.Run(kind, func(t *testing.T) {
			path := coreFile(t, "original")
			original, info, err := readOriginal(path)
			if err != nil {
				t.Fatal(err)
			}
			hook := func() {
				switch kind {
				case "content same size and time":
					if err := os.WriteFile(path, []byte("external"), 0640); err != nil {
						t.Fatal(err)
					}
					if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
						t.Fatal(err)
					}
				case "replacement inode":
					other := path + ".other"
					if err := os.WriteFile(other, original, 0640); err != nil {
						t.Fatal(err)
					}
					if err := os.Rename(other, path); err != nil {
						t.Fatal(err)
					}
				case "mode":
					if err := os.Chmod(path, 0600); err != nil {
						t.Fatal(err)
					}
				case "symlink":
					other := path + ".other"
					if err := os.Rename(path, other); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(other, path); err != nil {
						t.Fatal(err)
					}
				case "hardlink":
					if err := os.Link(path, path+".link"); err != nil {
						t.Fatal(err)
					}
				case "growth":
					if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 1024), 0640); err != nil {
						t.Fatal(err)
					}
				}
			}
			applied, err := atomicWrite(path, original, []byte("replacement"), info, writerHooks{beforeCheck: hook})
			if err == nil || applied {
				t.Fatalf("conflict %s not detected: %t %v", kind, applied, err)
			}
			if coreContents(t, path) == "replacement" {
				t.Fatal("overwrote competing editor")
			}
			entries, _ := os.ReadDir(filepath.Dir(path))
			for _, entry := range entries {
				if strings.Contains(entry.Name(), "mdsql") {
					t.Fatalf("leaked artifact %s", entry.Name())
				}
			}
		})
	}
}

func TestCoreWriterLockAndPostSync(t *testing.T) {
	if !atomicWritesSupported {
		t.Skip("atomic writes unavailable")
	}
	path := coreFile(t, "old")
	original, info, err := readOriginal(path)
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(filepath.Dir(path), ".note.md.mdsql.lock")
	if err := os.WriteFile(lock, []byte("owned by someone else"), 0600); err != nil {
		t.Fatal(err)
	}
	if applied, err := atomicWrite(path, original, []byte("new"), info, writerHooks{}); err == nil || applied {
		t.Fatal("failed to respect existing lock")
	}
	if coreContents(t, lock) != "owned by someone else" {
		t.Fatal("changed someone else's lock")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	applied, err := atomicWrite(path, original, []byte("new"), info, writerHooks{syncDirectory: func(*os.File) error { return errors.New("injected sync error") }})
	if !applied || err == nil || coreContents(t, path) != "new" {
		t.Fatalf("post-sync result %t %v", applied, err)
	}
	coreNoArtifacts(t, path)
}

func TestCoreRejectSources(t *testing.T) {
	stmt := coreStmt(t, "INSERT INTO lines (text) VALUES ('new')")
	for _, path := range []string{"", "-", "*.md", t.TempDir(), filepath.Join(t.TempDir(), "missing.md")} {
		if _, err := Execute(path, stmt, Options{DryRun: true}); err == nil {
			t.Fatalf("accepted %q", path)
		}
	}
	path := coreFile(t, "old")
	alias := filepath.Join(t.TempDir(), "alias.md")
	if err := os.Symlink(path, alias); err == nil {
		if _, err := Execute(alias, stmt, Options{DryRun: true}); err == nil {
			t.Fatal("accepted symlink")
		}
	}
	alias = filepath.Join(t.TempDir(), "hard.md")
	if err := os.Link(path, alias); err == nil {
		info, _ := os.Stat(path)
		if regularInfo(info) != nil {
			if _, err := Execute(path, stmt, Options{DryRun: true}); err == nil {
				t.Fatal("accepted hardlink")
			}
		}
	}
}

func TestCorePatchAndDiff(t *testing.T) {
	for _, patches := range [][]bytePatch{{{0, 2, nil}, {1, 3, nil}}, {{1, 1, []byte("a")}, {1, 1, []byte("b")}}, {{-1, 0, nil}}} {
		if _, err := applyPatches([]byte("abc"), patches); err == nil {
			t.Fatal("invalid patches accepted")
		}
	}
	got, err := applyPatches([]byte("abcdef"), []bytePatch{{4, 5, []byte("long")}, {0, 1, []byte("A")}})
	if err != nil || string(got) != "Abcdlongf" {
		t.Fatalf("patch %q %v", got, err)
	}
	diff := unifiedDiff("note.md", []byte("a\r\nb"), []byte("a\r\nc"))
	want := "--- note.md\n+++ note.md\n@@ -1,2 +1,2 @@\n a\r\n-b\n\\ No newline at end of file\n+c\n\\ No newline at end of file\n"
	if diff != want {
		t.Fatalf("diff %q want %q", diff, want)
	}
	if got := unifiedDiff("x", nil, []byte("x\n")); !strings.Contains(got, "@@ -0,0 +1,1 @@") || strings.Contains(got, "No newline") {
		t.Fatalf("empty diff %q", got)
	}
	if unifiedDiff("x", []byte("x"), []byte("x")) != "" {
		t.Fatal("nonempty no-op diff")
	}
}

func TestCoreDiffCompact(t *testing.T) {
	before := []byte(strings.Repeat("unchanged\n", 5000) + "old\n" + strings.Repeat("tail\n", 5000))
	after := []byte(strings.Replace(string(before), "old\n", "new\n", 1))
	got := unifiedDiff("note.md", before, after)
	if !strings.Contains(got, "@@ -4998,7 +4998,7 @@") || len(got) > 200 {
		t.Fatalf("diff was not compact: %.250s", got)
	}
}

func TestCoreEmptyLogicalLines(t *testing.T) {
	for _, tc := range []struct{ raw, text, want string }{
		{"", "\n", "\n"},
		{"old", "\n", "old\n\n"},
		{"old", "a\n\n", "old\na\n\n"},
		{"old\r\n", "\r\n", "old\r\n\r\n"},
	} {
		stmt := &query.WriteStatement{Operation: "INSERT", Table: "lines", Values: map[string]query.Value{"text": query.NewString(tc.text)}}
		got, m, c, err := planText([]byte(tc.raw), stmt, coreDoc(t, tc.raw))
		if err != nil || string(got) != tc.want || m != 1 || c != 1 {
			t.Fatalf("empty logical line: %q, %d, %d, %v; want %q", got, m, c, err, tc.want)
		}
	}
}

func TestCoreUnclosedRawHTML(t *testing.T) {
	for _, raw := range []string{"<script>\nconsole.log('hello')", "<pre>\ncode", "<STYLE>\nbody {}", "<textarea>\ntext"} {
		stmt := coreStmt(t, "INSERT INTO tasks (text) VALUES ('new')")
		if _, _, _, err := planText([]byte(raw), stmt, coreDoc(t, raw)); err == nil {
			t.Fatalf("accepted unfinished raw HTML: %q", raw)
		}
	}
	got, _, _ := corePlan(t, "<script>\ncode\n</script>", "INSERT INTO tasks (text) VALUES ('new')")
	if !strings.HasSuffix(got, "\n\n- [ ] new") {
		t.Fatalf("closed HTML: %q", got)
	}
}

func TestCoreLiteralSpecialFilenames(t *testing.T) {
	for _, name := range []string{"note[1].md", "note*.md", "note?.md"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
			if strings.ContainsAny(name, "*?") {
				continue
			}
			t.Fatal(err)
		}
		result, err := Execute(path, coreStmt(t, "INSERT INTO lines (text) VALUES ('new')"), Options{DryRun: true})
		if err != nil || result.Matched != 1 || result.Changed != 1 || result.Applied {
			t.Fatalf("literal filename %q: %#v %v", name, result, err)
		}
		if coreContents(t, path) != "old" {
			t.Fatal("dry-run wrote literal filename")
		}
		coreNoArtifacts(t, path)
	}
}
