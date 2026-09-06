package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIParenthesizedSourcesAndExplain(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a (1),b.md")
	second := filepath.Join(dir, "quote's.md")
	for path, content := range map[string]string{first: "# First\n", second: "# Second\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sources := "'" + first + "', \"" + second + "\""
	for _, sourceClause := range []string{sources, "(" + sources + ")"} {
		sql := "SELECT title FROM headings IN " + sourceClause + " ORDER BY title"
		code, out, errOut := captureFeedbackRun(t, []string{"--json", sql})
		if code != 0 || out != "[{\"title\":\"First\"},{\"title\":\"Second\"}]\n" || errOut != "" {
			t.Fatalf("SQL=%q code=%d stdout=%q stderr=%q", sql, code, out, errOut)
		}
	}
	code, out, errOut := captureFeedbackRun(t, []string{"--explain", "SELECT title FROM headings IN (" + sources + ") ORDER BY title"})
	want := "SELECT title FROM headings IN '" + first + "', '" + strings.ReplaceAll(second, "'", `\'`) + "' ORDER BY title\n"
	if code != 0 || out != want || errOut != "" {
		t.Fatalf("explain code=%d stdout=%q want=%q stderr=%q", code, out, want, errOut)
	}
}

func TestCLIMalformedSourcesFailBeforeStat(t *testing.T) {
	for _, sourceClause := range []string{"()", "('does-not-exist.md'", "('does-not-exist.md',)", "('does-not-exist.md' 'other.md')"} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", "SELECT title FROM headings IN " + sourceClause})
		if code != 1 || out != "" || !strings.Contains(errOut, "query parse error") || !strings.Contains(errOut, "file specification") || strings.Contains(errOut, "stat ") {
			t.Fatalf("sources=%q code=%d stdout=%q stderr=%q", sourceClause, code, out, errOut)
		}
	}
}
