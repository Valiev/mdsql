package main

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUniqueExpressionNamesAcrossFormats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	if err := os.WriteFile(path, []byte("- [ ] Text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"json", "jsonl", "csv", "tsv", "grouped", "text"} {
		code, out, errOut := captureFeedbackRun(t, []string{"--root", dir, "--format", format, "SELECT file, len(text), len(file) FROM tasks", path})
		if code != 0 || errOut != "" {
			t.Fatalf("%s: %d %q %q", format, code, out, errOut)
		}
		switch format {
		case "json", "jsonl":
			var rows []map[string]any
			if format == "jsonl" {
				out = "[" + strings.TrimSpace(out) + "]"
			}
			if err := json.Unmarshal([]byte(out), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0]["len"] != float64(4) || rows[0]["len_2"] != float64(7) {
				t.Fatalf("%s: %v", format, rows)
			}
		case "csv", "tsv":
			r := csv.NewReader(strings.NewReader(out))
			if format == "tsv" {
				r.Comma = '\t'
			}
			rows, err := r.ReadAll()
			if err != nil || len(rows) != 2 || strings.Join(rows[0], ",") != "file,len,len_2" || strings.Join(rows[1], ",") != "note.md,4,7" {
				t.Fatalf("%s: %v %v", format, rows, err)
			}
		case "grouped":
			if out != "len\tlen_2\n@\"note.md\"\n4\t7\n" {
				t.Fatalf("grouped: %q", out)
			}
		case "text":
			if out != "file=note.md\tlen=4\tlen_2=7\n" {
				t.Fatalf("text: %q", out)
			}
		}
	}
}

func TestExplicitDuplicateAliasesFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"SELECT line AS a, level AS a FROM headings", "SELECT count(*) AS a, count(*) AS a FROM headings"} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, path})
		if code != 1 || out != "" || !strings.Contains(errOut, `duplicate output alias "a"`) {
			t.Fatalf("%s: %d %q %q", sql, code, out, errOut)
		}
	}
}
