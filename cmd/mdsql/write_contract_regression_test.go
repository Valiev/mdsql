package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/edit"
)

// These tests deliberately use run() for both writes and subsequent reads.
// captureFeedbackRun redirects process-wide streams, so do not parallelize them.
func contractRun(t *testing.T, root, file, sql string, flags ...string) string {
	t.Helper()
	args := append([]string{"--root", root, "--json", sql, file}, flags...)
	code, out, stderr := captureFeedbackRun(t, args)
	if code != 0 || stderr != "" {
		t.Fatalf("run(%q): exit=%d stdout=%q stderr=%q", args, code, out, stderr)
	}
	return out
}

func contractReport(t *testing.T, out string, applied bool, matched, changed int) edit.Result {
	t.Helper()
	var result edit.Result
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("write report %q: %v", out, err)
	}
	if result.Applied != applied || result.Matched != matched || result.Changed != changed {
		t.Fatalf("report=%+v; want applied=%t matched=%d changed=%d", result, applied, matched, changed)
	}
	return result
}

func contractRows(t *testing.T, root, file, sql string) []map[string]any {
	t.Helper()
	out := contractRun(t, root, file, sql)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("SELECT output %q: %v", out, err)
	}
	return rows
}

func contractRead(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func contractBytes(t *testing.T, file string, want []byte) {
	t.Helper()
	if got := contractRead(t, file); !bytes.Equal(got, want) {
		t.Fatalf("file bytes differ:\n got %q\nwant %q", got, want)
	}
}

func contractFixture(t *testing.T, name, source string) (string, string) {
	t.Helper()
	root := t.TempDir()
	file := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return root, file
}

func TestWriteContractRegressionSequentialMarkdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only")
	}
	const name = "[draft].md"
	const original = "\ufeff---\r\n# metadata\r\nstage: old # retain stage comment\r\nowner: 'synthetic'\r\n# adjacent comment\r\n---\r\n# Inbox\r\n\r\nIntro.\r\n\r\n## Existing\r\nExisting body.\r\n\r\n# Tail\r\nTail body.\r\n"
	root, file := contractFixture(t, name, original)
	const insert = "INSERT INTO sections (line, level, title, body) VALUES (8, 2, 'Results', 'Result preface.\n\n### Detail\nDetail body.')"
	contractReport(t, contractRun(t, root, name, insert), true, 1, 1)
	rows := contractRows(t, root, name, "SELECT file, title, start, level, parent FROM sections WHERE title = 'Results'")
	want := []map[string]any{{"file": name, "title": "Results", "start": float64(8), "level": float64(2), "parent": "Inbox"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("explicit line=8 must put the heading at physical line 8: got=%v want=%v", rows, want)
	}

	contractReport(t, contractRun(t, root, name, "INSERT INTO tasks (section, text) VALUES ('Results', 'Parent task')"), true, 1, 1)
	rows = contractRows(t, root, name, "SELECT line, text, section, state FROM tasks WHERE text = 'Parent task'")
	if len(rows) != 1 || rows[0]["section"] != "Results" || rows[0]["state"] != "todo" {
		t.Fatalf("inserted task not reread in section own content: %v", rows)
	}
	parentLine, ok := rows[0]["line"].(float64)
	if !ok {
		t.Fatalf("task line is not numeric: %v", rows)
	}
	contractReport(t, contractRun(t, root, name, fmt.Sprintf("INSERT INTO tasks (parent_line, text) VALUES (%d, 'Child task')", int(parentLine))), true, 1, 1)
	rows = contractRows(t, root, name, "SELECT text, section, depth FROM tasks ORDER BY line")
	want = []map[string]any{
		{"text": "Parent task", "section": "Results", "depth": float64(0)},
		{"text": "Child task", "section": "Results", "depth": float64(1)},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("parent/child SELECT: got=%v want=%v", rows, want)
	}
	data := string(contractRead(t, file))
	if !strings.Contains(data, "- [ ] Parent task\r\n  - [ ] Child task\r\n") || strings.Index(data, "Child task") >= strings.Index(data, "### Detail") {
		t.Fatalf("child must follow parent before the child heading: %q", data)
	}

	const body = "before\n```\n````\n~~~\nafter"
	contractReport(t, contractRun(t, root, name, "INSERT INTO code (section, lang, code) VALUES ('Detail', 'text', '"+body+"')"), true, 1, 1)
	rows = contractRows(t, root, name, "SELECT lang, code, section FROM code")
	if len(rows) != 1 || rows[0]["lang"] != "text" || rows[0]["section"] != "Detail" {
		t.Fatalf("internal fences split or misplaced inserted code: %v", rows)
	}
	code, ok := rows[0]["code"].(string)
	if !ok || strings.TrimSuffix(strings.ReplaceAll(code, "\r\n", "\n"), "\n") != body {
		t.Fatalf("code payload changed: %q; want %q", code, body)
	}
	before := contractRead(t, file)
	update := "UPDATE tasks SET state = 'done' WHERE file = '[draft].md' AND text = 'Child task'"
	preview := contractReport(t, contractRun(t, root, name, update, "--dry-run", "--expect-rows=1"), false, 1, 1)
	contractBytes(t, file, before)
	applied := contractReport(t, contractRun(t, root, name, update, "--expect-rows=1", "--expect-sha256="+preview.BaseSHA256), true, 1, 1)
	if preview.ResultSHA256 != applied.ResultSHA256 {
		t.Fatalf("preview/apply hashes differ: preview=%+v applied=%+v", preview, applied)
	}
	rows = contractRows(t, root, name, "SELECT file, text, state FROM tasks WHERE state = 'done'")
	want = []map[string]any{{"file": name, "text": "Child task", "state": "done"}}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("SELECT did not reread UPDATE: got=%v want=%v", rows, want)
	}
	final := contractRead(t, file)
	if !bytes.HasPrefix(final, []byte(strings.Split(original, "# Inbox")[0]+"# Inbox\r\n")) || bytes.Count(final, []byte("\r\nIntro.\r\n\r\n")) != 1 || !bytes.HasSuffix(final, []byte("\r\n## Existing\r\nExisting body.\r\n\r\n# Tail\r\nTail body.\r\n")) {
		t.Fatalf("unrelated original prefix/suffix changed: %q", final)
	}
	withoutCRLF := bytes.ReplaceAll(final, []byte("\r\n"), nil)
	if bytes.ContainsAny(withoutCRLF, "\r\n") || bytes.Count(final, []byte("\ufeff")) != 1 {
		t.Fatalf("BOM/CRLF preservation failed: %q", final)
	}
}

func TestWriteContractRegressionScalarFrontmatter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only")
	}
	const original = "\ufeff---\r\n# before\r\nstage: 'draft'  # inline\r\n# adjacent\r\nowner: 'synthetic'\r\n---\r\n# Body\r\nKeep this body.\r\n"
	root, file := contractFixture(t, "note.md", original)
	insert := "INSERT INTO frontmatter_values (key, value) VALUES ('estimate', 1.25)"
	preview := contractReport(t, contractRun(t, root, "note.md", insert, "--dry-run", "--expect-rows=1"), false, 1, 1)
	contractBytes(t, file, []byte(original))
	if preview.Diff == "" || preview.BaseSHA256 == preview.ResultSHA256 {
		t.Fatalf("preview must report the planned change: %+v", preview)
	}
	contractReport(t, contractRun(t, root, "note.md", insert), true, 1, 1)
	want := strings.Replace(original, "owner: 'synthetic'\r\n", "owner: 'synthetic'\r\nestimate: 1.25\r\n", 1)
	contractBytes(t, file, []byte(want))

	contractReport(t, contractRun(t, root, "note.md", "UPDATE frontmatter_values SET value = 'ready' WHERE key = 'stage' AND value = 'draft'", "--expect-rows=1"), true, 1, 1)
	want = strings.Replace(want, "stage: 'draft'", "stage: 'ready'", 1)
	contractBytes(t, file, []byte(want))
	contractReport(t, contractRun(t, root, "note.md", "UPDATE frontmatter_values SET value = 2.5 WHERE key = 'estimate' AND value = 1.25", "--expect-rows=1"), true, 1, 1)
	want = strings.Replace(want, "estimate: 1.25", "estimate: 2.5", 1)
	contractBytes(t, file, []byte(want))
	rows := contractRows(t, root, "note.md", "SELECT file, fm.stage AS stage, fm.estimate AS estimate, fm.owner AS owner FROM frontmatter")
	expected := []map[string]any{{"file": "note.md", "stage": "ready", "estimate": 2.5, "owner": "synthetic"}}
	if !reflect.DeepEqual(rows, expected) {
		t.Fatalf("scalar projections: got=%v want=%v", rows, expected)
	}
	for _, sql := range []string{
		"UPDATE frontmatter_values SET value = 'ready' WHERE key = 'stage'",
		"UPDATE frontmatter_values SET value = 2.5 WHERE key = 'estimate' AND value = 2.5",
	} {
		result := contractReport(t, contractRun(t, root, "note.md", sql, "--expect-rows=1", "--dry-run"), false, 1, 0)
		if result.BaseSHA256 != result.ResultSHA256 {
			t.Fatalf("no-op changed report hash: %+v", result)
		}
		contractBytes(t, file, []byte(want))
	}
}

func TestWriteContractRegressionGuardsAndMatchedCounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only")
	}
	const original = "\ufeff# Tasks\r\n\r\n- [ ] First\r\n- [x] Second\r\n"
	root, file := contractFixture(t, "note.md", original)
	const update = "UPDATE tasks SET state = 'done' WHERE file = 'note.md'"
	for _, flags := range [][]string{{"--expect-rows=1"}, {"--dry-run", "--expect-rows=1"}} {
		args := append([]string{"--root", root, "--json", update, "note.md"}, flags...)
		code, out, stderr := captureFeedbackRun(t, args)
		if code != 1 || out != "" || stderr == "" {
			t.Fatalf("guard run(%q): exit=%d stdout=%q stderr=%q", args, code, out, stderr)
		}
		contractBytes(t, file, []byte(original))
	}
	for _, suffix := range []string{" LIMIT", " LIMIT 1", " LIMIT nope", " LIMIT -1", " LIMIT 1 trailing"} {
		code, out, stderr := captureFeedbackRun(t, []string{"--root", root, "--json", update + suffix, "note.md"})
		if code != 1 || out != "" || stderr == "" {
			t.Errorf("malformed UPDATE %q: exit=%d stdout=%q stderr=%q", update+suffix, code, out, stderr)
		}
		contractBytes(t, file, []byte(original))
	}
	preview := contractReport(t, contractRun(t, root, "note.md", update, "--dry-run", "--expect-rows=2"), false, 2, 1)
	contractBytes(t, file, []byte(original))
	contractReport(t, contractRun(t, root, "note.md", update, "--expect-rows=2"), true, 2, 1)
	want := []byte(strings.Replace(original, "- [ ] First", "- [x] First", 1))
	contractBytes(t, file, want)
	hash := sha256.Sum256(want)
	if preview.ResultSHA256 != hex.EncodeToString(hash[:]) {
		t.Fatalf("preview hash %q differs from actual bytes", preview.ResultSHA256)
	}
	contractReport(t, contractRun(t, root, "note.md", "UPDATE tasks SET state = 'todo' WHERE text = 'absent'", "--expect-rows=0", "--dry-run"), false, 0, 0)
	contractBytes(t, file, want)
}

func TestWriteContractRegressionFrontmatterLists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only")
	}
	const original = "---\nitems:\n  - one\n  - \"[[Old]]\" # keep\n---\nbody\n"
	root, file := contractFixture(t, "note.md", original)
	insert := "INSERT INTO frontmatter_values (key, value) VALUES ('items', '[[New]]')"
	contractReport(t, contractRun(t, root, "note.md", insert, "--expect-rows=1"), true, 1, 1)
	want := "---\nitems:\n  - one\n  - \"[[Old]]\" # keep\n  - \"[[New]]\"\n---\nbody\n"
	contractBytes(t, file, []byte(want))
	contractReport(t, contractRun(t, root, "note.md", insert, "--expect-rows=1"), false, 1, 0)
	update := "UPDATE frontmatter_values SET value = '[[Current]]' WHERE key = 'items' AND value = '[[Old]]'"
	contractReport(t, contractRun(t, root, "note.md", update, "--expect-rows=1"), true, 1, 1)
	want = strings.Replace(want, "\"[[Old]]\"", "\"[[Current]]\"", 1)
	contractBytes(t, file, []byte(want))
	rows := contractRows(t, root, "note.md", "SELECT value FROM frontmatter_values WHERE key = 'items' ORDER BY value")
	expected := []map[string]any{{"value": "[[Current]]"}, {"value": "[[New]]"}, {"value": "one"}}
	if !reflect.DeepEqual(rows, expected) {
		t.Fatalf("list rows: got=%v want=%v", rows, expected)
	}
}

func TestWriteContractRegressionRelativeFileIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only")
	}
	for _, absolute := range []bool{false, true} {
		t.Run(fmt.Sprintf("absolute_input=%t", absolute), func(t *testing.T) {
			const identity = "notes/[draft].md"
			root, file := contractFixture(t, identity, "---\nstage: draft # keep\n---\n- [ ] Synthetic\n")
			previousDir, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(root); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chdir(previousDir); err != nil {
					t.Errorf("restore working directory: %v", err)
				}
			})
			root = "."
			input := identity
			if absolute {
				input = file
			}
			selectedIdentity := ""
			for _, tc := range []struct{ table, projection, where, update string }{
				{"tasks", "text", "text = 'Synthetic'", "UPDATE tasks SET state = 'done'"},
				{"frontmatter_values", "key", "key = 'stage'", "UPDATE frontmatter_values SET value = 'ready'"},
			} {
				rows := contractRows(t, root, input, "SELECT file, "+tc.projection+" FROM "+tc.table+" WHERE "+tc.where)
				if len(rows) != 1 {
					t.Fatalf("SELECT must return one row: %v", rows)
				}
				readIdentity, ok := rows[0]["file"].(string)
				if !ok || readIdentity == "" {
					t.Fatalf("SELECT must return a nonempty file identity: %v", rows)
				}
				if !absolute && readIdentity != identity {
					t.Fatalf("relative SELECT file=%q; want %q", readIdentity, identity)
				}
				if selectedIdentity != "" && readIdentity != selectedIdentity {
					t.Fatalf("SELECT tables disagree on file identity: %q vs %q", readIdentity, selectedIdentity)
				}
				selectedIdentity = readIdentity
				where := " WHERE file = '" + strings.ReplaceAll(readIdentity, "'", "''") + "' AND " + tc.where
				contractReport(t, contractRun(t, root, input, tc.update+where, "--expect-rows=1"), true, 1, 1)
			}
			contractBytes(t, file, []byte("---\nstage: ready # keep\n---\n- [x] Synthetic\n"))
			rows := contractRows(t, root, input, "SELECT file, state FROM tasks WHERE state = 'done'")
			if len(rows) != 1 || rows[0]["file"] != selectedIdentity {
				t.Fatalf("SELECT after relative file UPDATE: %v", rows)
			}
		})
	}
}

func TestWriteContractRegressionPreviewPortable(t *testing.T) {
	const original = "\ufeff---\r\nstage: draft # keep\r\n---\r\n# Tasks\r\n\r\n- [ ] Synthetic\r\n"
	root, file := contractFixture(t, "note.md", original)
	for _, sql := range []string{
		"INSERT INTO tasks (section, text) VALUES ('Tasks', 'Preview only')",
		"UPDATE tasks SET state = 'done' WHERE text = 'Synthetic'",
		"INSERT INTO frontmatter_values (key, value) VALUES ('estimate', 1.25)",
		"UPDATE frontmatter_values SET value = 'ready' WHERE key = 'stage'",
	} {
		result := contractReport(t, contractRun(t, root, "note.md", sql, "--dry-run", "--expect-rows=1"), false, 1, 1)
		if result.Diff == "" || result.BaseSHA256 == result.ResultSHA256 {
			t.Fatalf("dry-run must describe the proposed edit for %q: %+v", sql, result)
		}
		contractBytes(t, file, []byte(original))
	}
}
