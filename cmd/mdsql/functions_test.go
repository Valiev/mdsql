package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRejectsUnknownFunctionsBeforeReadingRows(t *testing.T) {
	for _, content := range []string{"", "# Demo\n"} {
		path := filepath.Join(t.TempDir(), "synthetic.md")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			sql  string
			name string
		}{
			{"SELECT substr(title, 1, 3) FROM headings", "substr"},
			{"SELECT title FROM headings WHERE regex(title, 'x')", "regex"},
			{"SELECT title FROM headings WHERE false AND regex(title, 'x')", "regex"},
			{"SELECT title FROM headings WHERE true OR regex(title, 'x')", "regex"},
			{"SELECT title FROM headings ORDER BY substr(title, 1, 3)", "substr"},
		} {
			code, out, errOut := captureFeedbackRun(t, []string{"--json", tc.sql, path})
			if code != 1 || out != "" || !strings.Contains(errOut, `unknown function "`+tc.name+`"`) || !strings.Contains(errOut, "available functions:") {
				t.Fatalf("content=%q query=%q code=%d stdout=%q stderr=%q", content, tc.sql, code, out, errOut)
			}
		}
	}
}
