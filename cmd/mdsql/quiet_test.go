package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuietSuppressesWarningsButPreservesErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.md")
	if err := os.WriteFile(path, []byte("---\nbroken: [\n---\n# Visible\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, quiet := range []string{"-q", "--quiet"} {
		code, out, errOut := captureFeedbackRun(t, []string{quiet, "--json", "SELECT title FROM headings", path})
		if code != 0 || out != "[{\"title\":\"Visible\"}]\n" || errOut != "" {
			t.Fatalf("%s: %d %q %q", quiet, code, out, errOut)
		}
		code, out, errOut = captureFeedbackRun(t, []string{quiet, "--strict", "--json", "SELECT title FROM headings", path})
		if code != 2 || out != "" || !strings.Contains(errOut, "invalid frontmatter") {
			t.Fatalf("strict %s: %d %q %q", quiet, code, out, errOut)
		}
		code, out, errOut = captureFeedbackRun(t, []string{quiet, "--json", "SELECT unknown_function(title) FROM headings", path})
		if code != 1 || out != "" || !strings.Contains(errOut, "unknown function") {
			t.Fatalf("query %s: %d %q %q", quiet, code, out, errOut)
		}
	}
	path = maxRowsInput(t, 2)
	code, out, errOut := captureFeedbackRun(t, []string{"-q", "--strict", "--json", "--max-rows=1", "SELECT title FROM headings", path})
	if code != 4 || out != "[{\"title\":\"Row001\"}]\n" || !strings.Contains(errOut, "truncated, 1 of 2 rows") {
		t.Fatalf("truncation: %d %q %q", code, out, errOut)
	}
}
