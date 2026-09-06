package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIGroupedAndNoHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic.md")
	if err := os.WriteFile(path, []byte("# First\n## Second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		format, sql, want string
		noHeader          bool
	}{
		{"grouped", "SELECT file, title, line FROM headings", "title\tline\n@\"synthetic.md\"\nFirst\t1\nSecond\t2\n", false},
		{"grouped", "SELECT file FROM headings", "@\"synthetic.md\"\n", false},
		{"grouped", "SELECT file, title FROM headings WHERE level = 6", "", false},
		{"csv", "SELECT title, line FROM headings", "First,1\nSecond,2\n", true},
		{"tsv", "SELECT title, level FROM headings", "First\t1\nSecond\t2\n", true},
		{"csv", "SELECT title FROM headings WHERE level = 6", "", true},
	} {
		args := []string{"--root", dir, "--format", tt.format, tt.sql, path}
		if tt.noHeader {
			args = append(args, "--no-header")
		}
		code, out, errOut := captureFeedbackRun(t, args)
		if code != 0 || out != tt.want || errOut != "" {
			t.Fatalf("format=%s SQL=%s: code=%d stdout=%q stderr=%q", tt.format, tt.sql, code, out, errOut)
		}
	}
	code, out, errOut := captureFeedbackRun(t, []string{"--root", dir, "--format", "grouped", "--no-file", "SELECT file, title FROM headings", path})
	if code != 0 || out != "title\n@\"synthetic.md\"\nFirst\nSecond\n" || errOut != "" {
		t.Fatalf("grouped --no-file: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

func TestCLIGroupedRejectsMissingOrInvalidFile(t *testing.T) {
	for _, sql := range []string{"SELECT text FROM tasks", "SELECT file AS path FROM headings WHERE level = 6"} {
		code, out, errOut := captureFeedbackRun(t, []string{"--format", "grouped", sql, "does-not-exist.md"})
		if code != 1 || out != "" || !strings.Contains(errOut, "requires column file") {
			t.Fatalf("SQL=%s: code=%d stdout=%q stderr=%q", sql, code, out, errOut)
		}
	}
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("---\nname: synthetic\n---\n# First\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"SELECT line AS file FROM headings", "SELECT fm.missing AS file FROM frontmatter"} {
		code, out, errOut := captureFeedbackRun(t, []string{"--format", "grouped", sql, path})
		if code != 1 || out != "" || !strings.Contains(errOut, "requires string file") {
			t.Fatalf("SQL=%s: code=%d stdout=%q stderr=%q", sql, code, out, errOut)
		}
	}
}
