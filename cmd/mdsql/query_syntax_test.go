package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIUnicodeAliasesAndContextCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.md")
	if err := os.WriteFile(path, []byte("---\nданные:\n  имя: Пример\n---\n# Пример\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ sql, want string }{
		{"SELECT title AS Заголовок FROM headings", `[{"Заголовок":"Пример"}]`},
		{"SELECT fm.данные.имя AS Имя FROM frontmatter", `[{"Имя":"Пример"}]`},
		{"SELECT today() = today AS Дата, now() = now AS Время FROM headings", `[{"Время":true,"Дата":true}]`},
	} {
		code, out, errOut := captureFeedbackRun(t, []string{"--json", tc.sql, path})
		if code != 0 || out != tc.want+"\n" || errOut != "" {
			t.Fatalf("SQL=%q code=%d stdout=%q stderr=%q", tc.sql, code, out, errOut)
		}
	}
}

func TestCLIParseErrorShowsPositionBeforeLoadingFiles(t *testing.T) {
	code, out, errOut := captureFeedbackRun(t, []string{"--json", "SELECT title AS 1 FROM headings", "does-not-exist.md"})
	if code != 1 || out != "" || !strings.Contains(errOut, "at byte offset 16: expected alias after AS") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}
