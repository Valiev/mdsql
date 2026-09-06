package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIRejectsOverflowingIntegerLiteralsBeforeOutput(t *testing.T) {
	for _, content := range []string{"", "# Demo\n"} {
		path := filepath.Join(t.TempDir(), "synthetic.md")
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		for _, sql := range []string{
			"SELECT 9223372036854775808 FROM headings",
			"SELECT -9223372036854775809 FROM headings",
			"SELECT title FROM headings WHERE 9223372036854775807 = 9223372036854775808",
			"SELECT title FROM headings WHERE false AND line = 9223372036854775808",
			"SELECT title FROM headings LIMIT 9223372036854775808",
			"SELECT title FROM headings OFFSET 9223372036854775808",
		} {
			code, out, errOut := captureFeedbackRun(t, []string{"--json", sql, path})
			if code != 1 || out != "" || !strings.Contains(errOut, "at byte offset") || !strings.Contains(errOut, "out of range") {
				t.Fatalf("content=%q SQL=%q code=%d stdout=%q stderr=%q", content, sql, code, out, errOut)
			}
		}
	}
}

func TestCLIIntegerBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("# Demo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := captureFeedbackRun(t, []string{"--json", "SELECT -9223372036854775808 AS lo, 9223372036854775807 AS hi FROM headings", path})
	if code != 0 || out != "[{\"hi\":9223372036854775807,\"lo\":-9223372036854775808}]\n" || errOut != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}
