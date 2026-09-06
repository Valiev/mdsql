package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIFunctionArityFailsWithoutRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"SELECT head(raw) FROM headings", "SELECT lower(1, 2) FROM headings",
		"SELECT today(1) FROM headings", "SELECT coalesce() FROM headings",
		"SELECT title FROM headings WHERE false AND has_tag()",
		"SELECT title FROM headings ORDER BY upper()",
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, path})
		if code != 1 || out != "" || !strings.Contains(errOut, "query parse error") || !strings.Contains(errOut, "at byte offset") || !strings.Contains(errOut, "signature:") {
			t.Fatalf("SQL=%q code=%d stdout=%q stderr=%q", sql, code, out, errOut)
		}
	}
}

func TestCLIFunctionTypeWarningAndStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("# First\n# Second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"SELECT lower(level) AS value FROM headings",
		"SELECT title FROM headings WHERE at(title)",
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, path})
		if code != 0 || out == "" || strings.Count(errOut, "mdsql: warning:") != 1 || !strings.Contains(errOut, "signature:") {
			t.Fatalf("SQL=%q code=%d stdout=%q stderr=%q", sql, code, out, errOut)
		}
		code, out, errOut = captureFeedbackRun(t, []string{"--json", "--strict", sql, path})
		if code != 1 || out != "" || !strings.Contains(errOut, "signature:") {
			t.Fatalf("strict SQL=%q code=%d stdout=%q stderr=%q", sql, code, out, errOut)
		}
	}
}
