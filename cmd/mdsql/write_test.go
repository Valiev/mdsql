package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Valiev/mdsql/internal/edit"
	"github.com/Valiev/mdsql/internal/query"
)

func TestWriteDispatchOnlySQLSlot(t *testing.T) {
	sql := "INSERT INTO tasks(text) VALUES ('new')"
	for _, tc := range []struct {
		args []string
		want int
	}{
		{[]string{sql, "note.md"}, 0},
		{[]string{"--dry-run", "query", "--json", sql, "note.md"}, 3},
		{[]string{"--root", "query", "QuErY", "-- expect\n" + sql, "note.md"}, 3},
		{[]string{"/* lead */ -- comment\n" + sql, "note.md"}, 0},
		{[]string{"--version\n" + sql, "note.md"}, 0},
		{[]string{"SELECT 'INSERT INTO tasks' FROM lines", sql}, -1},
		{[]string{"query", "SELECT 'UPDATE' FROM lines", sql}, -1},
		{[]string{"--root", sql, "SELECT text FROM tasks", "note.md"}, -1},
		{[]string{"--null", sql, "SELECT text FROM tasks", "note.md"}, -1},
		{[]string{"tasks", "--tag", sql, "note.md"}, -1},
		{[]string{"grep", sql, "note.md"}, -1},
		{[]string{"query", "--root", sql, "SELECT text FROM tasks"}, -1},
		{[]string{"SELECT 'x' FROM lines", "--root", sql}, -1},
		{[]string{"INSERTION", "note.md"}, -1},
		{[]string{"UPDATE_other", "note.md"}, -1},
	} {
		got, _ := writeSQLIndex(tc.args)
		if got != tc.want {
			t.Errorf("args=%q index=%d want=%d", tc.args, got, tc.want)
		}
	}
}

func TestWriteArgsOptions(t *testing.T) {
	sql := "UPDATE tasks SET state='done' WHERE line=1"
	hash := strings.Repeat("A", 64)
	args := []string{"--root", "--help", "query", "--expect-rows=0", sql, "--dry-run", "--expect-sha256", hash, "--json", "--format=text", "--format", "json", "note.md"}
	index, priority := writeSQLIndex(args)
	cfg, err := parseWriteArgs(args, index)
	if err != nil || priority != "" || cfg.sql != sql || cfg.file != "note.md" || cfg.root != "--help" || cfg.format != "json" || !cfg.opts.DryRun || cfg.opts.ExpectRows == nil || *cfg.opts.ExpectRows != 0 || cfg.opts.ExpectSHA256 != strings.ToLower(hash) {
		t.Fatalf("config=%#v priority=%q error=%v", cfg, priority, err)
	}
	cfg, err = parseWriteArgs([]string{sql, "query"}, 0)
	if err != nil || cfg.file != "query" || cfg.opts.ExpectRows != nil {
		t.Fatalf("positional query: %#v %v", cfg, err)
	}
}

func TestWriteArgsRejectUnsupportedFlagsAndSources(t *testing.T) {
	sql := "INSERT INTO lines(text) VALUES ('new')"
	for _, suffix := range [][]string{
		{}, {"a.md", "b.md"}, {"-"},
		{"note.md", "--csv"}, {"note.md", "--strict"}, {"note.md", "--quiet"},
		{"note.md", "--max-rows", "1"}, {"note.md", "--include", "*.md"},
		{"note.md", "--cache"}, {"note.md", "--explain"}, {"note.md", "--clear-cache"},
		{"note.md", "-fjson"}, {"note.md", "--format=csv"}, {"note.md", "--format="},
		{"note.md", "--dry-run=true"}, {"note.md", "--json=true"},
		{"note.md", "--expect-rows=-1"}, {"note.md", "--expect-rows=1.5"},
		{"note.md", "--expect-rows=9999999999999999999999999"},
		{"note.md", "--expect-sha256="}, {"note.md", "--expect-sha256=bad"},
		{"note.md", "--expect-sha256=" + strings.Repeat("z", 64)},
		{"note.md", "--root"}, {"note.md", "--expect-rows"}, {"note.md", "--expect-sha256"},
	} {
		args := append([]string{sql}, suffix...)
		if _, err := parseWriteArgs(args, 0); err == nil {
			t.Errorf("accepted args=%q", args)
		}
	}
}

func TestWriteHelpVersionPriority(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--help", "INSERT malformed"}, "INSERT/UPDATE"},
		{[]string{"--dry-run", "query", "UPDATE tasks SET state='", "note.md", "--help"}, "INSERT/UPDATE"},
		{[]string{"query", "-- bad\nUPDATE malformed", "--format=csv", "--version"}, "mdsql " + appVersion},
		{[]string{"UPDATE malformed", "--help", "--version"}, "INSERT/UPDATE"},
		{[]string{"UPDATE malformed", "--version", "--help"}, "mdsql " + appVersion},
		{[]string{"--expect-rows=bad", "UPDATE malformed", "-V"}, "mdsql " + appVersion},
	} {
		code, out, stderr := captureFeedbackRun(t, tc.args)
		if code != 0 || !strings.Contains(out, tc.want) || stderr != "" {
			t.Errorf("args=%q code=%d stdout=%q stderr=%q", tc.args, code, out, stderr)
		}
	}
}

func TestWriteRegularFileAndRoot(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "synthetic.md")
	if err := os.WriteFile(file, []byte("# Synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveWriteFile(writeConfig{root: dir, file: "synthetic.md"})
	if err != nil || got != file {
		t.Fatalf("path=%q error=%v", got, err)
	}
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []writeConfig{
		{file: dir}, {file: link}, {root: file, file: file},
		{root: filepath.Join(dir, "missing"), file: file},
		{file: filepath.Join(dir, "missing.md")},
	} {
		if _, err := resolveWriteFile(cfg); err == nil {
			t.Errorf("accepted config=%#v", cfg)
		}
	}
}

func TestWriteCLIApplyPreviewAndGuards(t *testing.T) {
	for _, subcommand := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "query"}[subcommand], func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "synthetic.md")
			original := "- [ ] synthetic\n"
			if err := os.WriteFile(file, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--dry-run", "--expect-rows", "1"}
			if subcommand {
				args = append(args, "query")
			}
			args = append(args, "-- lead\nUPDATE tasks SET status='X' WHERE line=1;", file)
			code, out, stderr := captureFeedbackRun(t, args)
			if code != 0 || !strings.Contains(out, "--- ") || !strings.Contains(out, "+- [X] synthetic") || stderr != "" {
				t.Fatalf("preview: code=%d out=%q stderr=%q", code, out, stderr)
			}
			data, err := os.ReadFile(file)
			if err != nil || string(data) != original {
				t.Fatalf("preview wrote file: %q %v", data, err)
			}
			args[0] = "--json"
			if runtime.GOOS == "windows" {
				code, out, stderr = captureFeedbackRun(t, args)
				if code != 1 || out != "" || !strings.Contains(stderr, "atomic writes are unsupported on this platform") {
					t.Fatalf("apply support: code=%d out=%q stderr=%q", code, out, stderr)
				}
				return
			}
			code, out, stderr = captureFeedbackRun(t, args)
			var result edit.Result
			if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || stderr != "" || !result.Applied || result.Matched != 1 || result.Changed != 1 {
				t.Fatalf("apply: code=%d out=%q stderr=%q error=%v", code, out, stderr, err)
			}
			data, err = os.ReadFile(file)
			if err != nil || string(data) != "- [X] synthetic\n" {
				t.Fatalf("apply bytes=%q error=%v", data, err)
			}
			code, out, stderr = captureFeedbackRun(t, []string{"--expect-rows=0", "UPDATE tasks SET state='todo' WHERE line=1", file})
			if code != 1 || out != "" || stderr == "" {
				t.Fatalf("guard: code=%d out=%q stderr=%q", code, out, stderr)
			}
		})
	}
}

func TestWriteCLIRejectsSourcesWithoutWriting(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(file, []byte("original\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, files := range [][]string{{}, {"-"}, {file, file}, {filepath.Dir(file)}, {filepath.Join(filepath.Dir(file), "*.md")}} {
		args := append([]string{"INSERT INTO lines(text) VALUES ('added')"}, files...)
		code, out, stderr := captureFeedbackRun(t, args)
		if code != 1 || out != "" || stderr == "" {
			t.Errorf("args=%q code=%d out=%q stderr=%q", args, code, out, stderr)
		}
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "original\n" {
		t.Fatalf("bytes=%q error=%v", data, err)
	}
}

func TestWriteReportsAppliedResultOnError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(file, []byte("- [ ] synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, jsonOutput := range []bool{false, true} {
		args := []string{"UPDATE tasks SET state='done' WHERE line=1", file}
		if jsonOutput {
			args = append(args, "--json")
		}
		outFile, err := os.CreateTemp(t.TempDir(), "stdout")
		if err != nil {
			t.Fatal(err)
		}
		errFile, err := os.CreateTemp(t.TempDir(), "stderr")
		if err != nil {
			t.Fatal(err)
		}
		oldOut, oldErr := os.Stdout, os.Stderr
		os.Stdout, os.Stderr = outFile, errFile
		code, handled := runWrite(args, func(path string, stmt *query.WriteStatement, opts edit.Options) (*edit.Result, error) {
			return &edit.Result{File: path, Applied: true, Matched: 1, Changed: 1}, errors.New("directory sync failed after apply")
		})
		os.Stdout, os.Stderr = oldOut, oldErr
		outFile.Seek(0, io.SeekStart)
		errFile.Seek(0, io.SeekStart)
		out, _ := io.ReadAll(outFile)
		stderr, _ := io.ReadAll(errFile)
		outFile.Close()
		errFile.Close()
		if code != 1 || !handled || !strings.Contains(string(stderr), "directory sync failed after apply") {
			t.Fatalf("code=%d handled=%v stdout=%q stderr=%q", code, handled, out, stderr)
		}
		if jsonOutput {
			var result edit.Result
			if err := json.Unmarshal(out, &result); err != nil || !result.Applied || result.Matched != 1 || result.Changed != 1 {
				t.Fatalf("result=%#v error=%v", result, err)
			}
		} else if !strings.Contains(string(out), "applied: matched=1 changed=1") {
			t.Fatalf("stdout=%q", out)
		}
	}
}

func TestWriteDispatchLeavesCommentedSelectUntouched(t *testing.T) {
	for _, sql := range []string{
		"-- comment\nSELECT text FROM lines", "--version\nSELECT text FROM lines",
		"--\nSELECT text FROM lines", "/* comment */ SELECT text FROM lines", "-- comment",
	} {
		for _, prefix := range [][]string{nil, {"query"}, {"--json", "query"}} {
			args := append(append([]string{}, prefix...), sql, "INSERT INTO lines(text) VALUES ('not SQL')")
			if index, _ := writeSQLIndex(args); index != -1 {
				t.Errorf("classified SELECT source as mutation: args=%q index=%d", args, index)
			}
		}
	}
}

func TestWriteCLIInsertImmediatelyWithRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only")
	}
	for _, prefix := range [][]string{nil, {"query"}} {
		dir := t.TempDir()
		file := filepath.Join(dir, "synthetic.md")
		if err := os.WriteFile(file, []byte("# Synthetic\n\n"), 0600); err != nil {
			t.Fatal(err)
		}
		args := append(append([]string{}, prefix...), "--root", dir, "--expect-rows=1", "--json", "/* leading */ INSERT INTO tasks(text) VALUES ('new synthetic task');", "synthetic.md")
		code, out, stderr := captureFeedbackRun(t, args)
		var result edit.Result
		if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || stderr != "" || !result.Applied || result.Matched != 1 || result.Changed != 1 {
			t.Fatalf("insert: code=%d stdout=%q stderr=%q error=%v", code, out, stderr, err)
		}
		data, err := os.ReadFile(file)
		if err != nil || !strings.HasPrefix(string(data), "# Synthetic\n\n") || !strings.Contains(string(data), "- [ ] new synthetic task") {
			t.Fatalf("insert bytes=%q error=%v", data, err)
		}
		before := string(data)
		code, out, stderr = captureFeedbackRun(t, []string{"--expect-sha256=" + strings.Repeat("0", 64), "INSERT INTO tasks(text) VALUES ('blocked')", file})
		if code != 1 || out != "" || stderr == "" {
			t.Fatalf("sha guard: code=%d stdout=%q stderr=%q", code, out, stderr)
		}
		data, err = os.ReadFile(file)
		if err != nil || string(data) != before {
			t.Fatalf("sha guard changed bytes=%q error=%v", data, err)
		}
	}
}

func TestWriteCLIDecimalFrontmatterPredicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only")
	}
	file := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(file, []byte("---\nscore: 1.5\nother: 3.5\n---\n# Synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := captureFeedbackRun(t, []string{"--json", "--expect-rows=1", "query", "UPDATE frontmatter_values SET value=2.0 WHERE value=1.5", file})
	var result edit.Result
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || stderr != "" || !result.Applied || result.Matched != 1 || result.Changed != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q result=%#v error=%v", code, out, stderr, result, err)
	}
	code, out, stderr = captureFeedbackRun(t, []string{"--json", "SELECT key, value FROM frontmatter_values WHERE key='score'", file})
	var rows []struct {
		Key   string  `json:"key"`
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil || code != 0 || stderr != "" || len(rows) != 1 || rows[0].Value != 2 {
		t.Fatalf("updated value: code=%d stdout=%q stderr=%q error=%v", code, out, stderr, err)
	}
}

func TestWriteLiteralPathsNeverExpandGlobs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows supports preview only and forbids wildcard characters in filenames")
	}
	for _, name := range []string{"[draft].md", "literal*.md", "literal?.md"} {
		dir := t.TempDir()
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte("- [ ] literal\n"), 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"UPDATE tasks SET state='done' WHERE line=1", "--root", dir, name, "--expect-rows=1", "--json"}
		cfg, err := parseWriteArgs(args, 0)
		if err != nil || cfg.file != name {
			t.Fatalf("literal path rejected: %#v %v", cfg, err)
		}
		code, out, stderr := captureFeedbackRun(t, args)
		if code != 0 || !strings.Contains(out, `"applied":true`) || stderr != "" {
			t.Fatalf("name=%q code=%d stdout=%q stderr=%q", name, code, out, stderr)
		}
		data, err := os.ReadFile(file)
		if err != nil || string(data) != "- [x] literal\n" {
			t.Fatalf("literal bytes=%q error=%v", data, err)
		}
		// A matching file exists, but the wildcard spelling itself does not.
		args[3] = "*.md"
		code, out, stderr = captureFeedbackRun(t, args)
		if code != 1 || out != "" || stderr == "" {
			t.Fatalf("expanded wildcard: code=%d stdout=%q stderr=%q", code, out, stderr)
		}
	}
}

func TestWriteFileUnicodeEquivalentSpelling(t *testing.T) {
	for _, spellings := range [][2]string{
		{"caf\u00e9", "cafe\u0301"}, {"cafe\u0301", "caf\u00e9"},
	} {
		dir := t.TempDir()
		physicalDir := filepath.Join(dir, spellings[0])
		if err := os.Mkdir(physicalDir, 0700); err != nil {
			t.Fatal(err)
		}
		physical := filepath.Join(physicalDir, spellings[0]+".md")
		if err := os.WriteFile(physical, []byte("- [ ] unicode\n"), 0600); err != nil {
			t.Fatal(err)
		}
		for _, cfg := range []writeConfig{
			{root: dir, file: filepath.Join(spellings[1], spellings[1]+".md")},
			{file: filepath.Join(dir, spellings[1], spellings[1]+".md")},
		} {
			resolved, err := resolveWriteFile(cfg)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(resolved)
			if err != nil || string(data) != "- [ ] unicode\n" {
				t.Fatalf("resolved=%q bytes=%q error=%v", resolved, data, err)
			}
		}
		link := filepath.Join(dir, spellings[0]+"-link.md")
		if err := os.Symlink(physical, link); err != nil {
			t.Fatal(err)
		}
		if _, err := resolveWriteFile(writeConfig{root: dir, file: spellings[1] + "-link.md"}); err == nil {
			t.Fatal("Unicode resolution accepted a final symlink")
		}
	}
}

func TestWriteFilePredicateUsesSelectIdentity(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Error(err)
		}
	})
	for _, tc := range []struct {
		name string
		root string
		file string
	}{
		{"relative", "", "note.md"},
		{"absolute", "", filepath.Join(dir, "absolute.md")},
		{"root", "notes", "note.md"},
		{"outside-root", "notes", "../outside.md"},
		{"unicode-root", "caf\u00e9", "cafe\u0301.md"},
		{"literal-brackets", "notes", "[draft].md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			physical := tc.file
			if !filepath.IsAbs(physical) {
				physical = filepath.Join(tc.root, physical)
			}
			if err := os.MkdirAll(filepath.Dir(physical), 0700); err != nil {
				t.Fatal(err)
			}
			if tc.root != "" {
				if err := os.MkdirAll(tc.root, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(physical, []byte("- [ ] identity\n"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"--json"}
			if tc.root != "" {
				args = append(args, "--root", tc.root)
			}
			code, out, stderr := captureFeedbackRun(t, append(append([]string{}, args...), "SELECT file FROM tasks", tc.file))
			var rows []struct {
				File string `json:"file"`
			}
			if err := json.Unmarshal([]byte(out), &rows); err != nil || code != 0 || stderr != "" || len(rows) != 1 {
				t.Fatalf("SELECT code=%d stdout=%q stderr=%q error=%v", code, out, stderr, err)
			}
			predicate := strings.ReplaceAll(rows[0].File, "'", "\\'")
			sql := "UPDATE tasks SET state='done' WHERE file='" + predicate + "' AND line=1"
			writeArgs := append(append([]string{}, args...), "--expect-rows=1", sql, tc.file)
			if runtime.GOOS == "windows" {
				writeArgs = append(writeArgs, "--dry-run")
			}
			code, out, stderr = captureFeedbackRun(t, writeArgs)
			var result edit.Result
			wantApplied := runtime.GOOS != "windows"
			if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || stderr != "" || result.Applied != wantApplied || result.Matched != 1 || result.Changed != 1 {
				t.Fatalf("identity=%q UPDATE code=%d stdout=%q stderr=%q error=%v", rows[0].File, code, out, stderr, err)
			}
			data, err := os.ReadFile(physical)
			wantData := "- [x] identity\n"
			if !wantApplied {
				wantData = "- [ ] identity\n"
			}
			if err != nil || string(data) != wantData {
				t.Fatalf("wrong physical write: bytes=%q error=%v", data, err)
			}
		})
	}
}

func TestReadLiteralFilenameAndQuotedGlob(t *testing.T) {
	root := t.TempDir()
	for name, contents := range map[string]string{
		"[draft].md": "# Literal\n", "d.md": "# Decoy\n",
		"normal/a.md": "# First\n", "normal/b.md": "# Second\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct{ spec, want string }{
		{"[draft].md", `[{"title":"Literal"}]`},
		{"normal/*.md", `[{"title":"First"},{"title":"Second"}]`},
	} {
		// Each spec is one argv item, as with a shell-quoted glob.
		code, out, stderr := captureFeedbackRun(t, []string{"--root", root, "--json", "SELECT title FROM headings ORDER BY title", tc.spec})
		if code != 0 || strings.TrimSpace(out) != tc.want || stderr != "" {
			t.Errorf("spec=%q code=%d stdout=%q stderr=%q", tc.spec, code, out, stderr)
		}
	}
}

func TestWriteDotRootAbsoluteSourceIdentity(t *testing.T) {
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	file := filepath.Join(root, "notes", "[draft].md")
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("- [ ] Synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Error(err)
		}
	})
	code, out, stderr := captureFeedbackRun(t, []string{"--root", ".", "--json", "SELECT file FROM tasks WHERE text='Synthetic'", file})
	var rows []struct {
		File string `json:"file"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil || code != 0 || stderr != "" || len(rows) != 1 {
		t.Fatalf("SELECT code=%d stdout=%q stderr=%q error=%v", code, out, stderr, err)
	}
	// The same absolute source may remain outside the logical root on macOS
	// (/var versus /private/var). Preserve exactly what SELECT displayed.
	identity := strings.ReplaceAll(rows[0].File, "'", "\\'")
	sql := "UPDATE tasks SET state='done' WHERE file='" + identity + "' AND text='Synthetic'"
	writeArgs := []string{"--root", ".", "--expect-rows=1", "--json", sql, file}
	if runtime.GOOS == "windows" {
		writeArgs = append(writeArgs, "--dry-run")
	}
	code, out, stderr = captureFeedbackRun(t, writeArgs)
	var result edit.Result
	wantApplied := runtime.GOOS != "windows"
	if err := json.Unmarshal([]byte(out), &result); err != nil || code != 0 || stderr != "" || result.Matched != 1 || result.Changed != 1 || result.Applied != wantApplied {
		t.Fatalf("code=%d stdout=%q stderr=%q result=%#v error=%v", code, out, stderr, result, err)
	}
}
